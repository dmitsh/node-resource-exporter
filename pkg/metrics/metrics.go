package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	nodeResourceRequests  *prometheus.GaugeVec
	nodeResourceLimits    *prometheus.GaugeVec
	nodeResourceOccupancy *prometheus.GaugeVec
)

func New(nodeLabels []string) {
	scoreLabels := append([]string{"resource"}, nodeLabels...)
	labels := append([]string{"node"}, scoreLabels...)

	nodeResourceRequests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "node_resource_requests",
			Help: "Gauge of node resource requests.",
		}, labels)

	nodeResourceLimits = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "node_resource_limits",
			Help: "Gauge of node resource limits.",
		}, labels)

	nodeResourceOccupancy = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "node_resource_occupancy",
			Help: "Occupancy percentage of node resource.",
		}, labels)
}
