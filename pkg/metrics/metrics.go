package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	NodeLabelNames          []string
	NodeResourceRequests    *prometheus.GaugeVec
	NodeResourceLimits      *prometheus.GaugeVec
	NodeResourceUtilization *prometheus.GaugeVec
}

func New(nodeLabels []string) *Metrics {
	labels := append([]string{"node", "resource"}, nodeLabels...)

	return &Metrics{
		NodeLabelNames: nodeLabels,
		NodeResourceRequests: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "node_resource_requests",
				Help: "Gauge of node resource requests.",
			}, labels),

		NodeResourceLimits: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "node_resource_limits",
				Help: "Gauge of node resource limits.",
			}, labels),

		NodeResourceUtilization: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "node_resource_utilization",
				Help: "Utilization percentage of node resource.",
			}, labels),
	}
}
