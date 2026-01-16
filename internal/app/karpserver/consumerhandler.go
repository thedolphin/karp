package karpserver

import (
	"log/slog"
	"strconv"
	"strings"

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
		"claims", claimsToString(session.Claims()),
		"member-id", session.MemberID(),
		"generation-id", session.GenerationID(),
	)

	return nil
}

func (h *ConsumerGroupAggregatingHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	slog.Info("Leaved consumer group",
		"request-id", h.requestId,
		"claims", claimsToString(session.Claims()),
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

func claimsToString(claims map[string][]int32) string {

	var s strings.Builder

	f := true
	for topic, partitions := range claims {
		if f {
			f = false
		} else {
			s.WriteByte(' ')
		}

		s.WriteString(topic)
		s.WriteByte('(')

		for i, partition := range partitions {
			if i > 0 {
				s.WriteByte(',')
			}

			s.WriteString(strconv.FormatInt(int64(partition), 10))
		}

		s.WriteByte(')')
	}

	return s.String()
}
