package karpclient

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/IBM/sarama"
	"github.com/thedolphin/karp/internal/pkg/karputils"
	"github.com/thedolphin/karp/pkg/karp"
)

type Bridge struct {
	stream    *ConfigStream
	streamIdx int
}

func (b *Bridge) Run(ctx context.Context) {

	defer slog.Info("Bridge stopped", "number", b.streamIdx, "from", b.stream.Source, "to", b.stream.Sink)

	kafkaConfig := Config.Kafka[b.stream.Sink]
	saramaCfg := NewSaramaConfig(&kafkaConfig, b.stream.Partitioning)
	producer, err := sarama.NewSyncProducer(kafkaConfig.Brokers, saramaCfg)
	if err != nil {
		slog.Error("Error creating Kafka producer", "err", err)
		return
	}

	defer func() {
		slog.Info("Closing Kafka producer", "number", b.streamIdx, "from", b.stream.Source, "to", b.stream.Sink)
		if err = producer.Close(); err != nil {
			slog.Error("Closing Kafka producer", "err", err)
		}
	}()

	karpConfig := Config.Karp[b.stream.Source]

	karpClientConfig := &karp.ClientConfig{
		ClientID:        karpConfig.ClientID + "[" + strconv.Itoa(b.streamIdx) + "]",
		Endpoint:        karpConfig.Endpoint,
		UseTLS:          karpConfig.UseTLS,
		TrustServerCert: karpConfig.TrustServerCert,
		ClientCertPEM:   karpConfig.ClientCertPEM,
		ClientKeyPEM:    karpConfig.ClientKeyPEM,
		RootCAsPEM:      karpConfig.RootCAsPEM,
		Cluster:         karpConfig.Cluster,
		Group:           karpConfig.Group,
		User:            karpConfig.User,
		Password:        karpConfig.Password,
		Compression:     karpConfig.Compression,
	}

	karpClientConfig.Topics = make([]string, 0, len(b.stream.Topics))
	for topic := range b.stream.Topics {
		karpClientConfig.Topics = append(karpClientConfig.Topics, topic)
	}

	consumer, err := karp.NewClient(karpClientConfig)
	if err != nil {
		slog.Error("Error creating Karp consumer", "err", err)
		return
	}

	defer func() {
		slog.Info("Closing KARP consumer", "number", b.streamIdx, "from", b.stream.Source, "to", b.stream.Sink)
		if err = consumer.Close(); err != nil {
			slog.Error("Closing KARP consumer", "err", err)
		}
	}()

	cancelCtx, cancel := context.WithCancel(ctx)
	defer func() {
		slog.Info("Stopping bridge", "number", b.streamIdx, "from", b.stream.Source, "to", b.stream.Sink)
		cancel()
	}()

	slog.Info("Starting bridge", "number", b.streamIdx, "from", b.stream.Source, "to", b.stream.Sink)

	go func() {
		if err := consumer.Consume(cancelCtx); err != nil {
			slog.Error("Error consuming KARP stream", "err", err)
		}
	}()

	for msg := range consumer.Messages() {

		slog.Debug("Recieved message",
			"message", karputils.MessageFmt(msg.ID, msg.Topic, msg.Partition, msg.Offset))

		message := &sarama.ProducerMessage{
			Topic:     b.stream.Topics[msg.Topic],
			Key:       sarama.ByteEncoder(msg.Key),
			Value:     sarama.ByteEncoder(msg.Value),
			Headers:   make([]sarama.RecordHeader, len(msg.Headers)),
			Partition: msg.Partition,
			Timestamp: time.UnixMilli(msg.Timestamp),
		}

		for idx, header := range msg.Headers {
			message.Headers[idx] = sarama.RecordHeader{
				Key:   header.Key,
				Value: header.Value,
			}
		}

		if _, _, err := producer.SendMessage(message); err != nil {
			slog.Error("Error producing message", "err", err)
			return
		}

		if err = consumer.Commit(msg); err != nil {
			slog.Error("Error commiting offsets", "err", err)
			return
		}

		promProcessedMessages.WithLabelValues(
			karpConfig.Endpoint,
			b.stream.Sink,
			msg.Topic).Inc()
	}
}

func NewBridge(stream *ConfigStream, streamIdx int) *Bridge {
	return &Bridge{
		stream:    stream,
		streamIdx: streamIdx,
	}
}
