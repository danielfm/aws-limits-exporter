package core

import (
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/support"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

// Service limits check
var (
	checkID = "lN7RR0l7J9"
)

// NewSupportClient ...
func NewSupportClient() *SupportClientImpl {
	creds := credentials.NewChainCredentials(
		[]credentials.Provider{
			&credentials.EnvProvider{},
			&credentials.SharedCredentialsProvider{},
			&ec2rolecreds.EC2RoleProvider{
				Client: ec2metadata.New(session.Must(session.NewSession())),
			},
		})

	awsConfig := aws.NewConfig()
	awsConfig.WithCredentials(creds)

	// Trusted Advisor API does not work in every region, but we can use it
	// via the `us-east-1` region to get data from other regions
	awsConfig.WithRegion("us-east-1")

	sess := session.New(awsConfig)

	return &SupportClientImpl{
		SupportClient: support.New(sess),
	}
}

// RequestServiceLimitsRefreshLoop ...
func (client *SupportClientImpl) RequestServiceLimitsRefreshLoop() {
	input := &support.RefreshTrustedAdvisorCheckInput{
		CheckId: aws.String(checkID),
	}

	for {
		glog.Infof("Refreshing Trusted Advisor check '%s'...", checkID)
		output, err := client.SupportClient.RefreshTrustedAdvisorCheck(input)
		if err != nil {
			glog.Errorf("Error when requesting status refresh: %v", err)
			continue
		}

		waitMs := *output.Status.MillisUntilNextRefreshable

		glog.Infof("Refresh status is '%s', waiting %dms until the next refresh...", aws.StringValue(output.Status.Status), waitMs)
		time.Sleep(time.Duration(waitMs) * time.Millisecond)
	}
}

// DescribeServiceLimitsCheckResult ...
func (client *SupportClientImpl) DescribeServiceLimitsCheckResult() (*support.TrustedAdvisorCheckResult, error) {
	input := &support.DescribeTrustedAdvisorCheckResultInput{
		CheckId: aws.String(checkID),
	}

	output, err := client.SupportClient.DescribeTrustedAdvisorCheckResult(input)
	if err != nil {
		return nil, err
	}

	return output.Result, nil
}

// NewSupportExporter ...
func NewSupportExporter() *SupportExporter {
	client := NewSupportClient()

	return &SupportExporter{
		supportClient: client,
		metricsUsed:   map[string]*prometheus.Desc{},
		metricsLimit:  map[string]*prometheus.Desc{},
	}
}

// RequestServiceLimitsRefreshLoop ...
func (e *SupportExporter) RequestServiceLimitsRefreshLoop() {
	e.supportClient.RequestServiceLimitsRefreshLoop()
}

// Describe ...
func (e *SupportExporter) Describe(ch chan<- *prometheus.Desc) {
	result, err := e.supportClient.DescribeServiceLimitsCheckResult()
	if err != nil {
		glog.Errorf("Cannot retrieve Trusted Advisor check results data: %v", err)
	}

	for _, resource := range result.FlaggedResources {
		resourceID := aws.StringValue(resource.ResourceId)

		// Sanity check in order not to report the same metric more than once
		if _, ok := e.metricsUsed[resourceID]; ok {
			continue
		}

		serviceName := aws.StringValue(resource.Metadata[1])
		serviceNameLower := strings.ToLower(serviceName)
		glog.Infof("Refreshing Trusted Advisor check '%s'...", aws.StringValue(resource.Metadata[0]))
		// if aws.StringValue(resource.Region) == e.region {
		e.metricsUsed[resourceID] = newServerMetric(aws.StringValue(resource.Metadata[0]), serviceNameLower, "used_total", "Current used amount of the given resource.", []string{"resource"})
		e.metricsLimit[resourceID] = newServerMetric(aws.StringValue(resource.Metadata[0]), serviceNameLower, "limit_total", "Current limit of the given resource.", []string{"resource"})

		ch <- e.metricsUsed[resourceID]
		ch <- e.metricsLimit[resourceID]
		// }
	}
}

// Collect ...
func (e *SupportExporter) Collect(ch chan<- prometheus.Metric) {
	result, err := e.supportClient.DescribeServiceLimitsCheckResult()
	if err != nil {
		glog.Errorf("Cannot retrieve Trusted Advisor check results data: %v", err)
	}

	for _, resource := range result.FlaggedResources {
		resourceID := aws.StringValue(resource.ResourceId)

		// Sanity check in order not to report the same metric more than once
		metricUsed, ok := e.metricsUsed[resourceID]
		if !ok {
			continue
		}

		resourceName := aws.StringValue(resource.Metadata[2])

		metricLimit := e.metricsLimit[resourceID]
		limitValue, _ := strconv.ParseFloat(aws.StringValue(resource.Metadata[3]), 64)
		ch <- prometheus.MustNewConstMetric(metricLimit, prometheus.GaugeValue, limitValue, resourceName)

		usedValue, _ := strconv.ParseFloat(aws.StringValue(resource.Metadata[4]), 64)
		ch <- prometheus.MustNewConstMetric(metricUsed, prometheus.GaugeValue, usedValue, resourceName)
	}
}

func newServerMetric(region, subSystem, metricName, docString string, labels []string) *prometheus.Desc {
	return prometheus.NewDesc(
		prometheus.BuildFQName("aws", subSystem, metricName),
		docString, labels, prometheus.Labels{"region": region},
	)
}
