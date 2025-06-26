package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"flag"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/thedolphin/karp/internal/app/karpserver"
	"github.com/thedolphin/karp/internal/pkg/karputils"

	"go.uber.org/automaxprocs/maxprocs"

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

func getListener() (net.Listener, error) {
	var listener net.Listener
	var err error

	if karpserver.Config.UseTLS {

		var cert tls.Certificate

		if len(karpserver.Config.ServerCertPEM) == 0 || len(karpserver.Config.ServerCertKeyPEM) == 0 {

			slog.Warn("No certificate or key provided, generating self-signed certificate")

			cert, err = generateSelfSignedCert()
			if err != nil {
				return nil, fmt.Errorf("failed to generate self-signed cert: %v", err)
			}
		} else {
			cert, err = tls.X509KeyPair([]byte(karpserver.Config.ServerCertPEM), []byte(karpserver.Config.ServerCertKeyPEM))
			if err != nil {
				return nil, fmt.Errorf("failed to parse TLS certificates: %v", err)
			}
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"h2"}, // modern requirement, h2 == HTTP/2
		}

		listener, err = tls.Listen("tcp", karpserver.Config.Listen, tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS listener: %v", err)
		}

	} else {

		listener, err = net.Listen("tcp", karpserver.Config.Listen)
		if err != nil {
			return nil, fmt.Errorf("failed to create plain listener: %v", err)
		}
	}

	return listener, nil
}

func generateSelfSignedCert() (tls.Certificate, error) {

	host, err := os.Hostname()
	if err != nil {
		return tls.Certificate{}, err
	}

	priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(time.Now().UnixMicro()),
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour * 24 * 365),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	cert := tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  priv,
	}

	return cert, nil
}

func main() {

	defer slog.Info("See you!")

	maxprocs.Set()
	karputils.SetupLogger()
	karpserver.SetupPrometheus()

	configFile := flag.String("config", "", "configuration file path")
	flag.Parse()

	if len(*configFile) == 0 {
		slog.Error("Config file must be supplied")
		return
	}

	if err := karpserver.LoadConfig(*configFile); err != nil {
		slog.Error("Config file load error", "err", err)
		return
	}

	if err := karputils.SetLogLevel(karpserver.Config.LogLevel); err != nil {
		slog.Error("Could not set log level", "err", err)
		return
	}

	listener, err := getListener()
	if err != nil {
		slog.Error("Failed to start GRPC listener", "err", err, "listen", karpserver.Config.Listen)
		return
	}

	httpListener, err := net.Listen("tcp", karpserver.Config.HttpListen)
	if err != nil {
		slog.Error("Failed to start HTTP listener", "err", err, "listen", karpserver.Config.HttpListen)
		return
	}

	grpcServer := grpc.NewServer(
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    15 * time.Second,
			Timeout: 5 * time.Second,
		}),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	karpserver.RegisterKarpServer(grpcServer, ctx)
	reflection.Register(grpcServer)

	if karpserver.Config.LogLevel == "debug" {
		grpclog.SetLoggerV2(karputils.NewSlogGrpcLogger())
	}

	httpServer := &http.Server{
		Addr:    karpserver.Config.HttpListen,
		Handler: nil,
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		slog.Info("gRPC server is running", "listen", karpserver.Config.Listen)
		if err := grpcServer.Serve(listener); err != nil {
			slog.Error("gRPC server shutdown", "err", err)
		}
		cancel()
		wg.Done()
	}()

	go func() {
		slog.Info("HTTP server is running", "listen", karpserver.Config.HttpListen)
		if err := httpServer.Serve(httpListener); err != nil {
			slog.Error("HTTP server shutdown", "err", err)
		}
		cancel()
		wg.Done()
	}()

	// Main cycle
	<-ctx.Done()

	grpcServer.GracefulStop()
	httpServer.Shutdown(context.Background())

	wg.Wait()
}
