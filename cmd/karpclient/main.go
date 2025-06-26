package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/thedolphin/karp/internal/app/karpclient"
	"github.com/thedolphin/karp/internal/pkg/karputils"

	"go.uber.org/automaxprocs/maxprocs"
)

func main() {

	defer slog.Info("See you!")

	maxprocs.Set()
	karputils.SetupLogger()
	karpclient.SetupPrometheus()

	configFile := flag.String("config", "", "configuration file path")
	flag.Parse()

	if len(*configFile) == 0 {
		slog.Error("Config file must be supplied")
		return
	}

	err := karpclient.LoadConfig(*configFile)
	if err != nil {
		slog.Error("Config file load error", "err", err)
		return
	}

	if err := karputils.SetLogLevel(karpclient.Config.LogLevel); err != nil {
		slog.Error("Could not set log level", "err", err)
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	wg := &sync.WaitGroup{}

	for _, stream := range karpclient.Config.Streams {
		for streamIdx := range stream.Threads {

			wg.Add(1)
			go func() {

				slog.Info("Starting stream", "number", streamIdx, "from", stream.Source, "to", stream.Sink)

				for {
					karpclient.NewBridge(&stream, streamIdx).Run(ctx)

					select {
					case <-time.After(time.Duration(karpclient.Config.Retry)):
						continue

					case <-ctx.Done():
						slog.Info("Stopping stream", "number", streamIdx, "from", stream.Source, "to", stream.Sink)
						wg.Done()
						return
					}
				}
			}()
		}
	}

	httpListener, err := net.Listen("tcp", karpclient.Config.HttpListen)
	if err != nil {
		slog.Error("Failed to start HTTP listener", "err", err, "listen", karpclient.Config.HttpListen)
		return
	}

	httpServer := &http.Server{
		Addr:    karpclient.Config.HttpListen,
		Handler: nil,
	}

	wg.Add(1)
	go func() {
		slog.Info("HTTP server is running", "listen", karpclient.Config.HttpListen)
		if err := httpServer.Serve(httpListener); err != nil {
			slog.Warn("HTTP server shutdown", "err", err)
		}
		wg.Done()
	}()

	// Main cycle
	<-ctx.Done()

	httpServer.Shutdown(context.Background())

	wg.Wait()
}
