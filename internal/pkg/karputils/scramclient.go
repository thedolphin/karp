package karputils

// https://github.com/IBM/sarama/examples/sasl_scram_client/scram_client.go

import (
	"github.com/IBM/sarama"
	"github.com/xdg-go/scram"
)

type XDGSCRAMClient struct {
	hgf scram.HashGeneratorFcn
	*scram.ClientConversation
}

func (sc *XDGSCRAMClient) Begin(userName, password, authzID string) error {

	client, err := sc.hgf.NewClient(userName, password, authzID)
	if err != nil {
		return err
	}

	sc.ClientConversation = client.NewConversation()
	return nil
}

func XdgSha256ScramClientGenerator() sarama.SCRAMClient {
	return &XDGSCRAMClient{hgf: scram.SHA256}
}

func XdgSha512ScramClientGenerator() sarama.SCRAMClient {
	return &XDGSCRAMClient{hgf: scram.SHA512}
}
