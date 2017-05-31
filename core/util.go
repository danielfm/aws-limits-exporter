package core

import (
	"github.com/prometheus/client_golang/prometheus"
)

func newServerMetric(region, subSystem, metricName, docString string, labels []string) *prometheus.Desc {
	return prometheus.NewDesc(
		prometheus.BuildFQName("aws", subSystem, metricName),
		docString, labels, prometheus.Labels{"region": region},
	)
}
