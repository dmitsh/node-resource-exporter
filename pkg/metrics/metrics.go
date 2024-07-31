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
	NodeResourceScore     *prometheus.GaugeVec
}

func New(nodeLabels []string) *Metrics {
	scoreLabels := append([]string{"resource"}, nodeLabels...)
	labels := append([]string{"node"}, scoreLabels...)

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
		NodeResourceScore: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "node_resource_score",
				Help: "Occupancy score of node resource."}, scoreLabels),
	}
}

type ResourceScore struct {
	scores map[string]*Score
}

type Score struct {
	total float64
	count int64
}

func NewResourceScore() *ResourceScore {
	return &ResourceScore{
		scores: make(map[string]*Score),
	}
}

func (s *ResourceScore) Score(resource string, occ float64) float64 {
	score, ok := s.scores[resource]
	if !ok {
		score = &Score{total: occ, count: 1}
	} else {
		score.total += occ
		score.count++
	}
	s.scores[resource] = score

	return 100.0 * score.total / float64(score.count)
}
