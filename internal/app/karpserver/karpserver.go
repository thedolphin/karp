package karpserver

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/thedolphin/karp/pkg/karp"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type karpServer struct {
	karp.UnimplementedKarpServer
	reqestID atomic.Uint64
	ctx      context.Context
}

func (s *karpServer) Consume(stream grpc.BidiStreamingServer[karp.Ack, karp.Message]) error {

	requestId := s.reqestID.Add(1)

	var clientAddr string
	p, ok := peer.FromContext(stream.Context())
	if ok {
		clientAddr = p.Addr.String()
	}

	slog.Info("New Consume request", "request-id", requestId, "client", clientAddr)

	md, ok := metadata.FromIncomingContext(stream.Context())
	if !ok {
		return status.Error(codes.InvalidArgument, "failed to get metadata")
	}

	topicsMd := md["topic"]
	groupMd := md["group"]
	clusterMd := md["cluster"]
	userMd := md["user"]
	passwordMd := md["password"]
	clientIdMd := md["clientid"]

	var (
		user,
		password string
	)

	if len(topicsMd) < 1 {
		slog.Info("Client did not provide correct 'topic' header", "request-id", requestId)
		return status.Error(codes.InvalidArgument, "At least one 'topic' header must exist")
	}

	if len(groupMd) != 1 || len(groupMd[0]) == 0 {
		slog.Info("Client did not provide correct 'group' header", "request-id", requestId)
		return status.Error(codes.InvalidArgument, "Single 'group' header must exist and it must not be empty string")
	}

	if len(clusterMd) != 1 {
		slog.Info("Client did not provide correct 'cluster' header", "request-id", requestId)
		return status.Error(codes.InvalidArgument, "Single 'cluster' header must exist")
	}

	_, ok = Config.Clusters[clusterMd[0]]
	if !ok {
		slog.Info("Client provides unknown cluster", "request-id", requestId, "cluster", clusterMd[0])
		return status.Errorf(codes.InvalidArgument, "Cluster '%v' not defined", clusterMd[0])
	}

	if len(userMd) == 1 {
		user = userMd[0]
	} else if len(userMd) > 1 {
		slog.Info("Client did not provide correct 'user' header", "request-id", requestId)
		return status.Error(codes.InvalidArgument, "'user' header specified more than once")
	}

	if len(passwordMd) == 1 {
		password = passwordMd[0]
	} else if len(passwordMd) > 1 {
		slog.Info("Client did not provide correct 'password' header", "request-id", requestId)
		return status.Error(codes.InvalidArgument, "'password' header specified more than once")
	}

	saramaCfg := NewSaramaConfig(
		Config.Clusters[clusterMd[0]],
		user,
		password,
	)

	saramaCfg.ClientID = "karp"

	if len(clientIdMd) == 1 && len(clientIdMd[0]) > 0 {
		saramaCfg.ClientID += ":" + clientIdMd[0]
	} else {
		slog.Info("Client did not provide correct 'clientid' header", "request-id", requestId)
		return status.Error(codes.InvalidArgument, "'clientid' header must exist and it must not be empty string")
	}

	saramaCfg.ClientID += "@" + clientAddr

	handler := &ClientHandler{
		clientId:  clientIdMd[0],
		cluster:   clusterMd[0],
		requestId: requestId,
		group:     groupMd[0],
		user:      user,
		topics:    topicsMd,
		stream:    stream,
		saramaCfg: saramaCfg,
	}

	err := handler.Serve(s.ctx)

	slog.Info("Consume request complete", "request-id", requestId)
	return err
}

func RegisterKarpServer(grpcServer *grpc.Server, ctx context.Context) {
	karp.RegisterKarpServer(grpcServer, &karpServer{ctx: ctx})
}
