# **KA**fka g**RP**c bridge

![karp logo](assets/KARP_LOGO.svg)

# Motivation

* Publishing Kafka clusters over HTTP (in this case, HTTP/2 and gRPC)
* Synchronizing Kafka clusters across different data centers
* Simpler than Kafka REST API, thanks to bidirectional HTTP/2 streaming
* One connection = one Kafka client – disconnection gracefully and immediately terminates the Kafka client.
* Support multiple connections to a single topic with rebalancing
* Consistent repartition a topic on the fly using different methods
* Message filtering and mangling using Lua script

# Components

## 1. [`karpserver`](cmd/karpserver)

A gRPC server that streams messages from requested topics in configured Kafka clusters.
Supports multiple connections to the same topic within a single consumer group, with proper rebalancing handling.

## 2. [`karpclient`](cmd/karpclient)

A gRPC client that connects to the server and writes the message stream to configured Kafka clusters.

## 3. [`karp`](pkg/karp)

A public Go client library for connecting to Karp Server.

[`Usage example`](examples/karpclient.go)

## 4. [`karp.proto`](proto/karp.proto)

Protocol definition for other languages.

# Docker images

* [`karpserver`](https://hub.docker.com/repository/docker/thedolphin/karpserver/)
* [`karpclient`](https://hub.docker.com/repository/docker/thedolphin/karpclient/)