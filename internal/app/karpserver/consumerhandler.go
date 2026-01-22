package karpserver

import (
	"fmt"
	"log/slog"

	"github.com/IBM/sarama"
	"github.com/thedolphin/karp/internal/pkg/karputils"
	"github.com/thedolphin/luarunner"
)

type ConsumerGroupAggregatingHandler struct {
	session   sarama.ConsumerGroupSession
	luaScript string
	luaVMs    map[string]map[int32]*luarunner.LuaRunner
	client    *KarpClient
	messages  chan *sarama.ConsumerMessage
}

func (h *ConsumerGroupAggregatingHandler) Setup(session sarama.ConsumerGroupSession) error {
	h.session = session
	claims := session.Claims()
	slog.Info("Joined consumer group",
		"request-id", h.client.requestId,
		"claims", karputils.ClaimFmt(claims),
		"member-id", session.MemberID(),
		"generation-id", session.GenerationID(),
	)
	var err error

	if h.luaScript != "" {
		for topic, partitions := range claims {
			for _, partition := range partitions {
				h.luaVMs[topic][partition], err = luaInit(h.luaScript)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (h *ConsumerGroupAggregatingHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	slog.Info("Leaved consumer group",
		"request-id", h.client.clientId,
		"claims", karputils.ClaimFmt(session.Claims()),
		"member-id", session.MemberID(),
		"generation-id", session.GenerationID(),
	)

	for _, partitions := range h.luaVMs {
		for _, luaVM := range partitions {
			luaVM.Close()
		}
	}

	return nil
}

func (h *ConsumerGroupAggregatingHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {

	slog.Debug("Starting single partition consuming",
		"request-id", h.client.clientId,
		"topic", claim.Topic(),
		"partition", claim.Partition(),
	)

	defer slog.Debug("Stopped single partition consuming",
		"request-id", h.client.clientId,
		"topic", claim.Topic(),
		"partition", claim.Partition(),
	)

	var lua *luarunner.LuaRunner
	if h.luaScript != "" {
		lua = h.luaVMs[claim.Topic()][claim.Partition()]
	}

loop:
	for msg := range claim.Messages() {

		if lua != nil {
			// luaProcess changes msg.Value if necessary
			pass, err := luaProcess(lua, msg, h.client.user, h.client.group)
			if err != nil {
				return fmt.Errorf("error filtering message: %w", err)
			}
			if !pass {
				continue
			}
		}

		select {
		case h.messages <- msg:
		case <-session.Context().Done():
			break loop
		}
	}

	return nil
}

func (h *ConsumerGroupAggregatingHandler) Messages() <-chan *sarama.ConsumerMessage {
	return h.messages
}

func (h *ConsumerGroupAggregatingHandler) Close() {
	close(h.messages)
}

func NewConsumerGroupAggregatingHandler(client *KarpClient, luaScript string) *ConsumerGroupAggregatingHandler {
	return &ConsumerGroupAggregatingHandler{
		luaScript: luaScript,
		client:    client,
		messages:  make(chan *sarama.ConsumerMessage, 128),
	}
}
