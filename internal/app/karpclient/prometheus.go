package karpclient

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	promProcessedMessages = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karp_bridge_processed_messages",
			Help: "Number of processed Kafka messages",
		},
		[]string{"endpoint", "cluster", "topic"},
	)
)

func SetupPrometheus() {
	prometheus.MustRegister(
		promProcessedMessages,
	)

	http.Handle("/metrics", promhttp.Handler())
}
