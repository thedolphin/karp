package karpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/thedolphin/karp/internal/pkg/karputils"
	"github.com/thedolphin/karp/pkg/karp"
	"github.com/thedolphin/luarunner"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ClientHandler struct {
	requestId            uint64
	clientId             string
	cluster              string
	user                 string
	group                string
	consumerGroupHandler *ConsumerGroupAggregatingHandler
	consumer             sarama.ConsumerGroup
	messages             chan *sarama.ConsumerMessage
	stream               grpc.BidiStreamingServer[karp.Ack, karp.Message]
	topics               []string
	window               *FlowWindow
	saramaCfg            *sarama.Config
}

func (kch *ClientHandler) clientStreamHandler() error {

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
			"request-id", kch.requestId,
			"client-id", kch.clientId,
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

func (kch *ClientHandler) serverStreamHandler() error {

	var (
		lua *luarunner.LuaRunner
		err error
	)

	if luaScript, ok := LuaScripts[kch.cluster]; ok {
		lua, err = luaInit(luaScript)
		if err != nil {
			return fmt.Errorf("error initializing lua filter: %w", err)
		}

		defer lua.Close()
	}

	timer := time.NewTimer(Config.Ping)
	defer timer.Stop()

	for {

		id, err := kch.window.Next()
		if err != nil {
			return err
		}

		select {
		case msg, ok := <-kch.messages:
			if !ok {
				slog.Info("No more messages to send", "request-id", kch.requestId)
				return io.EOF
			}

			if lua != nil {

				// luaProcess изменяет msg.Value, если необходимо
				pass, err := luaProcess(lua, msg, kch.user, kch.group)
				if err != nil {
					slog.Error("Error filtering message: error executing script",
						"request-id", kch.requestId,
						"client-id", kch.clientId,
						"message", karputils.MessageFmt(id, msg.Topic, msg.Partition, msg.Offset),
						"err", err)

					return fmt.Errorf("error filtering message: %w", err)
				}

				if !pass {
					slog.Debug("Message filtered out",
						"request-id", kch.requestId,
						"client-id", kch.clientId,
						"message", karputils.MessageFmt(id, msg.Topic, msg.Partition, msg.Offset))

					continue
				}
			}

			slog.Debug("Sending message to client",
				"request-id", kch.requestId,
				"client-id", kch.clientId,
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

			promServedMessages.WithLabelValues(kch.clientId, kch.cluster, msg.Topic).Inc()

		case <-timer.C:
			slog.Debug("Sending PING to client",
				"request-id", kch.requestId,
				"client-id", kch.clientId,
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

func (kch *ClientHandler) kafkaConsumer(ctx context.Context) error {

	defer close(kch.messages)

	for {

		slog.Info("Starting consuming session", "request-id", kch.requestId)

		err := kch.consumer.Consume(ctx, kch.topics, kch.consumerGroupHandler)
		if err != nil {
			slog.Error("Consuming session stopped with error", "request-id", kch.requestId, "err", err)
			return fmt.Errorf("consuming stopped, err: %w", err)
		} else {
			slog.Info("Consuming session stopped with no error", "request-id", kch.requestId)
		}

		// If the context is canceled, it means there's an error somewhere.
		// Otherwise, it's a rebalance and we need to restart Consume().
		if err = ctx.Err(); err != nil {
			slog.Info("Consuming loop stopped", "request-id", kch.requestId)
			return io.EOF
		}
	}
}

func (kch *ClientHandler) Serve(ctx context.Context) error {

	var err error
	if kch.consumer, err = sarama.NewConsumerGroup(
		Config.Clusters[kch.cluster].Brokers,
		kch.group,
		kch.saramaCfg,
	); err != nil {
		slog.Error("Error creating consumer group", "request-id", kch.requestId, "err", err)
		return status.Error(codes.Internal, "KARP Server Internal Error")
	}

	kch.messages = make(chan *sarama.ConsumerMessage, 128)
	kch.consumerGroupHandler = NewConsumerGroupAggregatingHandler(kch.messages, kch.requestId)

	serveCtx, serveCancel := context.WithCancelCause(ctx)
	kch.window = NewFlowWindow(serveCtx, Config.WindowSize, Config.Timeout)

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		err := kch.clientStreamHandler()
		slog.Debug("Client stream handler stopped", "err", err)
		serveCancel(err)
		wg.Done()
	}()

	go func() {
		err := kch.serverStreamHandler()

		// if the stream is still open for sending
		if kch.stream.Context().Err() == nil {

			err := kch.stream.Send(&karp.Message{Done: true})
			if err != nil {
				slog.Error("Error sending Done to client",
					"request-id", kch.requestId,
					"client-id", kch.clientId,
					"err", err)
			}
		}

		slog.Debug("Server stream handler stopped", "err", err)
		serveCancel(err)

		// unblock kafka group consumer handler
		for range kch.messages {
		}

		wg.Done()
	}()

	err = kch.kafkaConsumer(serveCtx)
	slog.Debug("Kafka consumer stopped, waiting goroutines to finish", "err", err)

	waitCtx, waitCancel := context.WithTimeout(context.Background(), Config.Timeout)

	go func() {
		wg.Wait()
		waitCancel()
	}()

	<-waitCtx.Done()
	if waitCtx.Err() == context.DeadlineExceeded {
		slog.Warn("Deadline exceeded while waiting goroutines to finish")
	}

	// after we have received and committed the last ack
	kch.consumer.Close()

	err = context.Cause(serveCtx)
	if err != nil &&
		!errors.Is(err, io.EOF) && /* client sent END_STREAM */
		!errors.Is(err, context.Canceled) /* SIGTERM */ {
		slog.Error("Error while handling client", "request-id", kch.requestId, "err", err)
		return status.Error(codes.Internal, "KARP Server Internal Error")
	}

	return nil
}
