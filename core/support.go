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
	"github.com/aws/aws-sdk-go/service/support/supportiface"

	"github.com/golang/glog"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	checkId = "eW7HH0l7J9" // Service limits check
)

type SupportClientImpl struct {
	SupportClient supportiface.SupportAPI
}

type SupportClient interface {
	RequestServiceLimitsRefreshLoop()
	DescribeServiceLimitsCheckResult() (*support.TrustedAdvisorCheckResult, error)
}

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

func (client *SupportClientImpl) RequestServiceLimitsRefreshLoop() {
	input := &support.RefreshTrustedAdvisorCheckInput{
		CheckId: aws.String(checkId),
	}

	for {
		glog.Infof("Refreshing Trusted Advisor check '%s'...", checkId)
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

func (client *SupportClientImpl) DescribeServiceLimitsCheckResult() (*support.TrustedAdvisorCheckResult, error) {
	input := &support.DescribeTrustedAdvisorCheckResultInput{
		CheckId: aws.String(checkId),
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

func (e *SupportExporter) RequestServiceLimitsRefreshLoop() {
	e.supportClient.RequestServiceLimitsRefreshLoop()
}

func (e *SupportExporter) Describe(ch chan<- *prometheus.Desc) {
	result, err := e.supportClient.DescribeServiceLimitsCheckResult()
	if err != nil {
		glog.Errorf("Cannot retrieve Trusted Advisor check results data: %v", err)
	}

	for _, resource := range result.FlaggedResources {
		resourceId := aws.StringValue(resource.ResourceId)

		// Sanity check in order not to report the same metric more than once
		if _, ok := e.metricsUsed[resourceId]; ok {
			continue
		}

		serviceName := aws.StringValue(resource.Metadata[1])
		serviceNameLower := strings.ToLower(serviceName)

		if aws.StringValue(resource.Region) == e.region {
			e.metricsUsed[resourceId] = newServerMetric(e.region, serviceNameLower, "used_total", "Current used amount of the given resource.", []string{"resource"})
			e.metricsLimit[resourceId] = newServerMetric(e.region, serviceNameLower, "limit_total", "Current limit of the given resource.", []string{"resource"})

			ch <- e.metricsUsed[resourceId]
			ch <- e.metricsLimit[resourceId]
		}
	}
}

func (e *SupportExporter) Collect(ch chan<- prometheus.Metric) {
	result, err := e.supportClient.DescribeServiceLimitsCheckResult()
	if err != nil {
		glog.Errorf("Cannot retrieve Trusted Advisor check results data: %v", err)
	}

	for _, resource := range result.FlaggedResources {
		resourceId := aws.StringValue(resource.ResourceId)

		// Sanity check in order not to report the same metric more than once
		metricUsed, ok := e.metricsUsed[resourceId]
		if !ok {
			continue
		}

		resourceName := aws.StringValue(resource.Metadata[2])

		metricLimit := e.metricsLimit[resourceId]
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
