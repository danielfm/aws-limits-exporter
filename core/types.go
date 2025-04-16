package core

import (
	"github.com/aws/aws-sdk-go/service/support"
	"github.com/aws/aws-sdk-go/service/support/supportiface"
	"github.com/prometheus/client_golang/prometheus"
)

// SupportClientImpl ...
type SupportClientImpl struct {
	SupportClient supportiface.SupportAPI
	Region        string
}

// SupportClient ...
type SupportClient interface {
	RequestServiceLimitsRefreshLoop()
	DescribeServiceLimitsCheckResult(checkID string) (*support.TrustedAdvisorCheckResult, error)
	GetAvailableCheckIDs() ([]string, error)
}

// SupportExporter ...
type SupportExporter struct {
	supportClient SupportClient
	metricsRegion string
	metricsUsed   map[string]*prometheus.Desc
	metricsLimit  map[string]*prometheus.Desc
}
