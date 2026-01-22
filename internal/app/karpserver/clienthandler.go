package karpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/IBM/sarama"
	"github.com/thedolphin/karp/internal/pkg/karputils"
	"github.com/thedolphin/karp/pkg/karp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type KarpClient struct {
	requestId uint64
	clientId  string
	user      string
	group     string
	topics    []string
	cluster   string
}

type KarpClientHandler struct {
	client               *KarpClient
	consumerGroupHandler *ConsumerGroupAggregatingHandler
	consumer             sarama.ConsumerGroup
	stream               grpc.BidiStreamingServer[karp.Ack, karp.Message]
	window               *FlowWindow
	saramaCfg            *sarama.Config
}

func (kch *KarpClientHandler) clientStreamHandler() error {

	for {
		ack, err := kch.stream.Recv()
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("client stopped reading: %w", err)
			} else {
				return fmt.Errorf("error receiving message from client: %w", err)
			}
		}

		slog.Debug("Received ack from client",
			"request-id", kch.client.requestId,
			"client-id", kch.client.clientId,
			"message", karputils.MessageFmt(ack.ID, ack.Topic, ack.Partition, ack.Offset))

		kch.window.Chase(ack.ID)

		if ack.Topic == "" {
			continue
		}

		if kch.consumerGroupHandler.session != nil {
			kch.consumerGroupHandler.session.MarkOffset(ack.Topic, ack.Partition, ack.Offset+1, "")
		}
	}
}

func (kch *KarpClientHandler) serverStreamHandler() error {

	timer := time.NewTimer(Config.Ping)
	defer timer.Stop()

	for {

		id, err := kch.window.Next()
		if err != nil {
			return err
		}

		select {
		case msg, ok := <-kch.consumerGroupHandler.Messages():
			if !ok {
				slog.Info("No more messages to send, sending Done to client", "request-id", kch.client.requestId)
				err = kch.stream.Send(&karp.Message{Done: true})
				if err != nil {
					slog.Error("Error sending Done to client",
						"request-id", kch.client.requestId,
						"client-id", kch.client.clientId,
						"err", err)
				}
				return err
			}

			slog.Debug("Sending message to client",
				"request-id", kch.client.requestId,
				"client-id", kch.client.clientId,
				"message", karputils.MessageFmt(id, msg.Topic, msg.Partition, msg.Offset))

			headers := make([]*karp.MessageHeader, len(msg.Headers))
			for i, v := range msg.Headers {
				headers[i] = &karp.MessageHeader{
					Key:   v.Key,
					Value: v.Value,
				}
			}

			err = kch.stream.Send(&karp.Message{
				ID:        id,
				Headers:   headers,
				Timestamp: msg.Timestamp.UnixMilli(),
				Key:       msg.Key,
				Value:     msg.Value,
				Topic:     msg.Topic,
				Partition: msg.Partition,
				Offset:    msg.Offset,
			})

			if err != nil {
				return fmt.Errorf("error sending message: %w", err)
			}

			promServedMessages.WithLabelValues(kch.client.clientId, kch.client.cluster, msg.Topic).Inc()

		case <-timer.C:
			slog.Debug("Sending PING to client",
				"request-id", kch.client.requestId,
				"client-id", kch.client.clientId,
				"message id", id)

			err = kch.stream.Send(&karp.Message{
				ID: id,
			})

			if err != nil {
				return fmt.Errorf("error sending message: %w", err)
			}
		}

		timer.Reset(Config.Ping)
	}
}

func (kch *KarpClientHandler) kafkaConsumer(ctx context.Context) error {

	defer kch.consumerGroupHandler.Close()

	for {
		slog.Info("Starting consuming session", "request-id", kch.client.requestId)

		err := kch.consumer.Consume(ctx, kch.client.topics, kch.consumerGroupHandler)
		if err != nil {
			slog.Error("Consuming session stopped with error", "request-id", kch.client.requestId, "err", err)
			return fmt.Errorf("consuming stopped, err: %w", err)
		} else {
			slog.Info("Consuming session stopped with no error", "request-id", kch.client.requestId)
		}

		// If the context is canceled, it means there's an error somewhere.
		// Otherwise, it's a rebalance and we need to restart Consume().
		if err = ctx.Err(); err != nil {
			slog.Info("Consuming loop stopped", "request-id", kch.client.requestId)
			return err
		}
	}
}

func (kch *KarpClientHandler) Serve(ctx context.Context) error {

	var err error
	if kch.consumer, err = sarama.NewConsumerGroup(
		Config.Clusters[kch.client.cluster].Brokers,
		kch.client.group,
		kch.saramaCfg,
	); err != nil {
		slog.Error("Error creating consumer group", "request-id", kch.client.requestId, "err", err)
		return status.Errorf(codes.Internal, "KARP Server Internal Error: Error creating consumer group: %v", err)
	}

	serveGrp, serveCtx := errgroup.WithContext(ctx)
	kch.window = NewFlowWindow(serveCtx, Config.WindowSize, Config.Timeout)

	luaScript, _ := LuaScripts[kch.client.cluster]
	kch.consumerGroupHandler = NewConsumerGroupAggregatingHandler(kch.client, luaScript)

	// client sends close -> kafka consumer stops -> server goroutine stops
	// server goroutine stops -> kafka consumer stops -> clientStreamHandler HANGS waiting for client to close connection

	serveGrp.Go(func() error {
		err := kch.clientStreamHandler()
		slog.Debug("Client stream handler stopped", "err", err)
		return err
	})

	serveGrp.Go(func() error {
		err := kch.serverStreamHandler()
		slog.Debug("Server stream handler stopped", "err", err)
		return err
	})

	serveGrp.Go(func() error {
		err = kch.kafkaConsumer(serveCtx)
		slog.Debug("Kafka consumer stopped", "err", err)
		return err
	})

	err = serveGrp.Wait()

	kch.consumer.Close()

	if err != nil &&
		!errors.Is(err, io.EOF) && /* client sent END_STREAM */
		!errors.Is(err, context.Canceled) /* SIGTERM */ {
		// Hide wrapped TCP errors to avoid disclosing cluster configuration
		if errors.Is(err, sarama.ErrOutOfBrokers) {
			err = sarama.ErrOutOfBrokers
		}
		slog.Error("Error while handling client", "request-id", kch.client.requestId, "err", err)
		return status.Errorf(codes.Internal, "KARP Server Internal Error: %v", err)
	}

	return nil
}
