package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	NodeLabelNames        []string
	NodeResourceRequests  *prometheus.GaugeVec
	NodeResourceLimits    *prometheus.GaugeVec
	NodeResourceOccupancy *prometheus.GaugeVec
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

		NodeResourceOccupancy: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "node_resource_occupancy",
				Help: "Occupancy percentage of node resource.",
			}, labels),
	}
}
