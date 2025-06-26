package karpserver

import (
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/IBM/sarama"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/thedolphin/karp/internal/pkg/karputils"
)

type ServerConfigKafkaCluster struct {
	Brokers       []string `yaml:"brokers" env-required:"true"`
	SaslMechanism string   `yaml:"sasl"`
	TLS           bool     `yaml:"tls"`
	LuaScript     string   `yaml:"script"`
}

type ServerConfig struct {
	Listen           string                              `yaml:"listen" env-default:":50051"`
	HttpListen       string                              `yaml:"monitoring" env-default:":8080"`
	WindowSize       uint                                `yaml:"window" env-default:"4"`
	Ping             time.Duration                       `yaml:"ping" env-default:"5s"`
	Timeout          time.Duration                       `yaml:"timeout" env-default:"15s"`
	LogLevel         string                              `yaml:"loglevel" env-default:"info"`
	UseTLS           bool                                `yaml:"tls" env-default:"false"`
	ServerCertPEM    string                              `yaml:"cert"`
	ServerCertKeyPEM string                              `yaml:"key"`
	Clusters         map[string]ServerConfigKafkaCluster `yaml:"clusters"`
}

var Config = ServerConfig{}

// loaded and verified lua scripts
var LuaScripts = map[string]string{}

func NewSaramaConfig(kafka ServerConfigKafkaCluster, user, password string) *sarama.Config {

	saramaCfg := sarama.NewConfig()

	if kafka.TLS {
		saramaCfg.Net.TLS.Enable = true
	}

	saslMechanism := kafka.SaslMechanism
	if len(saslMechanism) > 0 { // default PLAIN
		switch saslMechanism {
		case "PLAIN":
			saramaCfg.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		case "SCRAM-SHA-256":
			saramaCfg.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
			saramaCfg.Net.SASL.SCRAMClientGeneratorFunc = karputils.XdgSha256ScramClientGenerator
		case "SCRAM-SHA-512":
			saramaCfg.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
			saramaCfg.Net.SASL.SCRAMClientGeneratorFunc = karputils.XdgSha512ScramClientGenerator
		}
	}

	if len(user) > 0 {
		saramaCfg.Net.SASL.Enable = true
		saramaCfg.Net.SASL.User = user

		if len(password) > 0 {
			saramaCfg.Net.SASL.Password = password
		}
	}

	return saramaCfg
}

func LoadConfig(configFile string) error {

	if err := cleanenv.ReadConfig(configFile, &Config); err != nil {
		return err
	}

	for name, cluster := range Config.Clusters {
		if len(cluster.SaslMechanism) > 0 &&
			!slices.Contains([]string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"}, cluster.SaslMechanism) {

			return fmt.Errorf("sasl mechanism expected to be SCRAM-SHA-256 or SCRAM-SHA-512, defaults to PLAIN, got: '%v' for cluster '%v'",
				cluster.SaslMechanism, name)
		}

		if len(cluster.LuaScript) > 0 {
			script, err := os.ReadFile(cluster.LuaScript)
			if err != nil {
				return err
			}

			lua, err := luaInit(string(script))

			if err != nil {
				return err
			}

			lua.Close()
			LuaScripts[name] = string(script)
		}
	}

	return nil
}
