package core

import (
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/support"
	"github.com/aws/aws-sdk-go/service/support/supportiface"

	"github.com/golang/glog"

	"github.com/prometheus/client_golang/prometheus"
)

type SupportClientImpl struct {
	SupportClient supportiface.SupportAPI
}

type SupportClient interface {
	DescribeServiceLimitsCheckResult() (*support.TrustedAdvisorCheckResult, error)
}

func NewSupportClient() *SupportClientImpl {
	creds := credentials.NewChainCredentials(
		[]credentials.Provider{
			&credentials.EnvProvider{},
			&credentials.SharedCredentialsProvider{},
		})

	awsConfig := aws.NewConfig()
	awsConfig.WithCredentials(creds)
	awsConfig.WithRegion("us-east-1")

	sess := session.New(awsConfig)

	return &SupportClientImpl{
		SupportClient: support.New(sess),
	}
}

func (client *SupportClientImpl) DescribeServiceLimitsCheckResult() (*support.TrustedAdvisorCheckResult, error) {
	input := &support.DescribeTrustedAdvisorCheckResultInput{
		CheckId: aws.String("eW7HH0l7J9"),
	}

	output, err := client.SupportClient.DescribeTrustedAdvisorCheckResult(input)
	if err != nil {
		return nil, err
	}

	return output.Result, nil
}

type SupportExporter struct {
	region        string
	supportClient SupportClient
	metricsUsed   map[string]*prometheus.Desc
	metricsLimit  map[string]*prometheus.Desc
}

func NewSupportExporter(region string) *SupportExporter {
	client := NewSupportClient()

	return &SupportExporter{
		region:        region,
		supportClient: client,
		metricsUsed:   map[string]*prometheus.Desc{},
		metricsLimit:  map[string]*prometheus.Desc{},
	}
}

func (e *SupportExporter) Describe(ch chan<- *prometheus.Desc) {
	if len(e.metricsUsed) == 0 {
		result, err := e.supportClient.DescribeServiceLimitsCheckResult()
		if err != nil {
			glog.Errorf("Cannot retrieve trusted advisor check results data: %v", err)
		}

		for _, resource := range result.FlaggedResources {
			resourceId := aws.StringValue(resource.ResourceId)

			if _, ok := e.metricsUsed[resourceId]; ok {
				continue
			}

			serviceName := aws.StringValue(resource.Metadata[1])
			serviceNameLower := strings.ToLower(serviceName)

			if aws.StringValue(resource.Region) == e.region {
				e.metricsUsed[resourceId] = NewServerMetric(e.region, serviceNameLower, "used_total", "Current used amount of the given resource.", []string{"resource"})
				e.metricsLimit[resourceId] = NewServerMetric(e.region, serviceNameLower, "limit_total", "Current limit of the given resource.", []string{"resource"})

				ch <- e.metricsUsed[resourceId]
				ch <- e.metricsLimit[resourceId]
			}
		}
	}
}

func (e *SupportExporter) Collect(ch chan<- prometheus.Metric) {
	result, err := e.supportClient.DescribeServiceLimitsCheckResult()
	if err != nil {
		glog.Errorf("Cannot retrieve trusted advisor check results data: %v", err)
	}

	for _, resource := range result.FlaggedResources {
		resourceId := aws.StringValue(resource.ResourceId)

		metricUsed, ok := e.metricsUsed[resourceId]
		if !ok {
			continue
		}
		metricLimit := e.metricsLimit[resourceId]

		resourceName := aws.StringValue(resource.Metadata[2])

		usedValue, _ := strconv.ParseFloat(aws.StringValue(resource.Metadata[4]), 64)
		limitValue, _ := strconv.ParseFloat(aws.StringValue(resource.Metadata[3]), 64)

		ch <- prometheus.MustNewConstMetric(metricLimit, prometheus.GaugeValue, limitValue, resourceName)
		ch <- prometheus.MustNewConstMetric(metricUsed, prometheus.GaugeValue, usedValue, resourceName)
	}
}

func NewServerMetric(region, subSystem, metricName, docString string, labels []string) *prometheus.Desc {
	return prometheus.NewDesc(
		prometheus.BuildFQName("aws", subSystem, metricName),
		docString, labels, prometheus.Labels{"region": region},
	)
}
