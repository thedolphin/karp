package karpserver

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	promServedMessages = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karp_server_served_messages",
			Help: "Number of Kafka messages served to client",
		},
		[]string{"client", "cluster", "topic"},
	)
)

func SetupPrometheus() {
	prometheus.MustRegister(
		promServedMessages,
	)

	http.Handle("/metrics", promhttp.Handler())
}
