package karpclient

import (
	"fmt"
	"slices"
	"time"

	"github.com/IBM/sarama"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/thedolphin/karp/internal/pkg/karputils"
)

type ConfigStream struct {
	Source       string            `yaml:"source" env-required:"true"`
	Sink         string            `yaml:"sink" env-required:"true"`
	Topics       map[string]string `yaml:"topics" env-required:"true"`
	Partitioning string            `yaml:"partitioning"`
	Threads      int               `yaml:"threads" env-default:"1"`
}

type ConfigKafkaCluster struct {
	Brokers       []string `yaml:"brokers" env-required:"true"`
	SaslMechanism string   `yaml:"sasl"`
	TLS           bool     `yaml:"tls"`
	User          string   `yaml:"user"`
	Password      string   `yaml:"password"`
	Idempotent    bool     `yaml:"idempotent"`
}

type ConfigKarpServer struct {
	ClientID        string `yaml:"clientid"`                     // identify client on Karp Server and Kafka Server (as a part of Member ID)
	Endpoint        string `yaml:"endpoint" env-required:"true"` // string containing address and port of Karp Server
	UseTLS          bool   `yaml:"tls"`                          // whether TLS is used when connecting to the Karp Server
	TrustServerCert bool   `yaml:"trust"`                        // skip server certificate verification if set to true
	ClientCertPEM   string `yaml:"clientcert"`                   // string containing client certificate in PEM format
	ClientKeyPEM    string `yaml:"clientkey"`                    // string containing client certificate key in PEM format
	RootCAsPEM      string `yaml:"rootca"`                       // string containing CA certificate in PEM format
	Cluster         string `yaml:"cluster" env-required:"true"`  // cluster name, configured on server, to connect to
	Group           string `yaml:"group" env-required:"true"`    // client Consumer Group name
	User            string `yaml:"user"`                         // Kafka user name
	Password        string `yaml:"password"`                     // Kafka user password
}

type ClientConfig struct {
	ClientID   string                        `yaml:"clientid" env-required:"true"`
	LogLevel   string                        `yaml:"loglevel" env-default:"info"`
	HttpListen string                        `yaml:"monitoring" env-default:":8080"`
	Retry      time.Duration                 `yaml:"retry" env-default:"5s"`
	Kafka      map[string]ConfigKafkaCluster `yaml:"kafka" env-required:"true"`
	Karp       map[string]ConfigKarpServer   `yaml:"karp" env-required:"true"`
	Streams    []ConfigStream                `yaml:"streams" env-required:"true"`
}

var Config ClientConfig

func NewSaramaConfig(kafka *ConfigKafkaCluster, partitioning string) *sarama.Config {

	saramaCfg := sarama.NewConfig()

	saramaCfg.Producer.Return.Successes = true

	if len(partitioning) > 0 {
		switch partitioning {
		case "hash": // default in sarama, no need to change
		case "roundrobin":
			saramaCfg.Producer.Partitioner = sarama.NewRoundRobinPartitioner
		case "source":
			fallthrough
		default:
			saramaCfg.Producer.Partitioner = sarama.NewManualPartitioner
		}
	}

	if kafka.TLS {
		saramaCfg.Net.TLS.Enable = true
	}

	if kafka.Idempotent {
		saramaCfg.Producer.Idempotent = true
		saramaCfg.Producer.RequiredAcks = sarama.WaitForAll // default: sarama.WaitForLocal
		saramaCfg.Net.MaxOpenRequests = 1                   // default: 5
	}

	if len(kafka.User) > 0 {
		saramaCfg.Net.SASL.Enable = true
		saramaCfg.Net.SASL.User = kafka.User

		if len(kafka.Password) > 0 {
			saramaCfg.Net.SASL.Password = kafka.Password
		}

		switch kafka.SaslMechanism {
		case "PLAIN": // default
			saramaCfg.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		case "SCRAM-SHA-256":
			saramaCfg.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
			saramaCfg.Net.SASL.SCRAMClientGeneratorFunc = karputils.XdgSha256ScramClientGenerator
		case "SCRAM-SHA-512":
			saramaCfg.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
			saramaCfg.Net.SASL.SCRAMClientGeneratorFunc = karputils.XdgSha512ScramClientGenerator
		}
	}

	return saramaCfg
}

func LoadConfig(configFile string) error {

	if err := cleanenv.ReadConfig(configFile, &Config); err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	for streamIdx, stream := range Config.Streams {
		if _, ok := Config.Kafka[stream.Sink]; !ok {
			return fmt.Errorf("sink '%v' has no definition in 'clusters' section", stream.Sink)
		}

		if _, ok := Config.Karp[stream.Source]; !ok {
			return fmt.Errorf("source '%v' has no definition in 'karp' section", stream.Source)
		}

		for srcTopic, dstTopic := range stream.Topics {
			if len(dstTopic) == 0 {
				stream.Topics[srcTopic] = srcTopic
			}
		}

		if len(stream.Partitioning) > 0 &&
			!slices.Contains([]string{"hash", "source", "roundrobin"}, stream.Partitioning) {

			return fmt.Errorf("'partitioning' expected to be 'hash', 'source' or 'roundrobin', defaults to 'source', got: '%v' for stream #%v",
				stream.Partitioning, streamIdx)
		}

		if stream.Threads < 1 {
			Config.Streams[streamIdx].Threads = 1
		}
	}

	for name, kafka := range Config.Kafka {
		if len(kafka.SaslMechanism) > 0 &&
			!slices.Contains([]string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"}, kafka.SaslMechanism) {

			return fmt.Errorf("sasl mechanism expected to be SCRAM-SHA-256 or SCRAM-SHA-512, defaults to PLAIN, got: '%v' for cluster '%v'",
				kafka.SaslMechanism, name)
		}
	}

	for name, karp := range Config.Karp {
		if len(karp.ClientID) == 0 {
			karp.ClientID = Config.ClientID
			Config.Karp[name] = karp
		}
	}

	return nil
}
