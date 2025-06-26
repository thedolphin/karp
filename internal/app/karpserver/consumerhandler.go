package karpserver

import (
	"log/slog"

	"github.com/IBM/sarama"
)

type ConsumerGroupAggregatingHandler struct {
	requestId uint64
	session   sarama.ConsumerGroupSession
	messages  chan<- *sarama.ConsumerMessage
}

func (h *ConsumerGroupAggregatingHandler) Setup(session sarama.ConsumerGroupSession) error {
	h.session = session
	slog.Info("Joined consumer group",
		"request-id", h.requestId,
		"claims", session.Claims(),
		"member-id", session.MemberID(),
		"generation-id", session.GenerationID(),
	)

	return nil
}

func (h *ConsumerGroupAggregatingHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	slog.Info("Leaved consumer group",
		"request-id", h.requestId,
		"claims", session.Claims(),
		"member-id", session.MemberID(),
		"generation-id", session.GenerationID(),
	)

	return nil
}

func (h *ConsumerGroupAggregatingHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {

	slog.Debug("Starting single partition consuming",
		"request-id", h.requestId,
		"topic", claim.Topic(),
		"partition", claim.Partition(),
	)

	for msg := range claim.Messages() {
		h.messages <- msg
	}

	slog.Debug("Stopped single partition consuming",
		"request-id", h.requestId,
		"topic", claim.Topic(),
		"partition", claim.Partition(),
	)

	return nil
}

func NewConsumerGroupAggregatingHandler(messages chan<- *sarama.ConsumerMessage, reqiestId uint64) *ConsumerGroupAggregatingHandler {
	return &ConsumerGroupAggregatingHandler{
		requestId: reqiestId,
		messages:  messages,
	}
}
