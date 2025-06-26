package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/thedolphin/karp/pkg/karp"
)

func main() {

	clientConfig := &karp.ClientConfig{
		ClientID: "karp_client",
		Endpoint: "karpserver:443",
		Cluster:  "primary",
		Group:    "karp_group",
		User:     "kafka_user",
		Password: "p@44w0rD",
		Topics:   []string{"test_topic"}}

	// Main context
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	client, err := karp.NewClient(clientConfig)
	if err != nil {
		log.Panicf("Error creating client: %v", err)
	}

	go func() {
		if err := client.Consume(ctx); err != nil {
			log.Printf("Error consuming KARP stream: %v", err)
		}
	}()

	// main cycle
	for msg := range client.Messages() {
		log.Printf("Recieved message: %v[%v]@%v", msg.Topic, msg.Partition, msg.Offset)

		// process message

		if err := client.Commit(msg); err != nil {
			log.Printf("Error sending acknowledgement")
			break
		}
	}

	// break Consume()
	cancel()

	// close connection
	client.Close()
}
