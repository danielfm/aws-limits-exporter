package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/support"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

// NewSupportClient creates a new SupportClientImpl for the given region
func NewSupportClient(region string) *SupportClientImpl {
	awsConfig := aws.NewConfig().WithRegion(region)
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		glog.Fatal(err)
	}
	return &SupportClientImpl{
		SupportClient: support.New(sess),
		Region:        region,
	}
}

// GetAvailableCheckIDs fetches all Trusted Advisor check IDs available in this partition/account
func (client *SupportClientImpl) GetAvailableCheckIDs() ([]string, error) {
	input := &support.DescribeTrustedAdvisorChecksInput{
		Language: aws.String("en"),
	}
	output, err := client.SupportClient.DescribeTrustedAdvisorChecks(input)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(output.Checks))
	for _, check := range output.Checks {
		if check.Id != nil {
			ids = append(ids, *check.Id)
		}
	}
	return ids, nil
}

// DescribeServiceLimitsCheckResult fetches the result for a specific Trusted Advisor check
func (client *SupportClientImpl) DescribeServiceLimitsCheckResult(checkID string) (*support.TrustedAdvisorCheckResult, error) {
	input := &support.DescribeTrustedAdvisorCheckResultInput{
		CheckId: aws.String(checkID),
	}
	output, err := client.SupportClient.DescribeTrustedAdvisorCheckResult(input)
	if err != nil {
		return nil, err
	}
	if output == nil || output.Result == nil {
		return nil, nil
	}
	return output.Result, nil
}

// RequestServiceLimitsRefreshLoop periodically refreshes all available TA checks
func (client *SupportClientImpl) RequestServiceLimitsRefreshLoop() {
	var waitMs int64 = 3600000 // 1 hour
	region := client.Region
	glog.Infof("Starting Trusted Advisor refresh loop for region: %s", region)

	for {
		checkIDs, err := client.GetAvailableCheckIDs()
		if err != nil {
			glog.Errorf("Failed to get available Trusted Advisor checks: %v", err)
			time.Sleep(time.Duration(waitMs) * time.Millisecond)
			continue
		}
		for _, checkID := range checkIDs {
			glog.Infof("Refreshing Trusted Advisor check '%s' in region: %s", checkID, region)
			input := &support.RefreshTrustedAdvisorCheckInput{
				CheckId: aws.String(checkID),
			}
			_, err := client.SupportClient.RefreshTrustedAdvisorCheck(input)
			if err != nil {
				glog.Errorf("Error when requesting status refresh for check '%s': %v", checkID, err)
				continue
			}

			result, err := client.DescribeServiceLimitsCheckResult(checkID)
			if err != nil {
				glog.Errorf("Failed to get check result for '%s': %v", checkID, err)
				continue
			}
			if result == nil {
				glog.Errorf("No result for check: %s", checkID)
				continue
			}
			glog.Infof("Check '%s' summary: Status: %s, FlaggedResources: %d, ResourcesProcessed: %d",
				checkID,
				aws.StringValue(result.Status),
				aws.Int64Value(result.ResourcesSummary.ResourcesFlagged),
				aws.Int64Value(result.ResourcesSummary.ResourcesProcessed),
			)
			for i, res := range result.FlaggedResources {
				if i >= 5 {
					glog.Infof("...only showing first 5 flagged resources")
					break
				}
				glog.Infof("  Resource[%d]: Region=%s | Status=%s | Metadata=%v",
					i,
					aws.StringValue(res.Region),
					aws.StringValue(res.Status),
					res.Metadata,
				)
			}
		}
		glog.Infof("Waiting %d minutes until the next refresh...", waitMs/60000)
		time.Sleep(time.Duration(waitMs) * time.Millisecond)
	}
}

// NewSupportExporter creates a new exporter for the given region
func NewSupportExporter(region string) *SupportExporter {
	return &SupportExporter{
		supportClient: NewSupportClient(region),
		metricsRegion: region,
		metricsUsed:   make(map[string]*prometheus.Desc),
		metricsLimit:  make(map[string]*prometheus.Desc),
	}
}

// Describe sends metric descriptors to Prometheus
func (e *SupportExporter) Describe(ch chan<- *prometheus.Desc) {
	// Dynamic metrics: nothing to send here
}

// Collect sends metric values to Prometheus
func (e *SupportExporter) Collect(ch chan<- prometheus.Metric) {
	checkIDs, err := e.supportClient.GetAvailableCheckIDs()
	if err != nil {
		glog.Errorf("Failed to get available Trusted Advisor checks: %v", err)
		return
	}
	for _, checkID := range checkIDs {
		result, err := e.supportClient.DescribeServiceLimitsCheckResult(checkID)
		if err != nil {
			glog.Errorf("Cannot retrieve Trusted Advisor check results data: %v", err)
			continue
		}
		if result == nil {
			glog.Errorf("No result for check: %s", checkID)
			continue
		}
		for _, res := range result.FlaggedResources {
			if len(res.Metadata) < 3 || res.Metadata[0] == nil || res.Metadata[1] == nil || res.Metadata[2] == nil {
				glog.Warningf("Resource metadata too short or nil for check %s: %v", checkID, res.Metadata)
				continue
			}
			resourceName := *res.Metadata[0]
			usedStr := *res.Metadata[1]
			limitStr := *res.Metadata[2]
			used, err1 := parseFloat(usedStr)
			limit, err2 := parseFloat(limitStr)
			if err1 != nil || err2 != nil {
				glog.Warningf("Cannot parse used/limit for check %s, resource %s: %v/%v", checkID, resourceName, err1, err2)
				continue
			}
			serviceName := parseServiceNameFromCheck(result)
			region := e.metricsRegion

			metricKey := fmt.Sprintf("%s_%s_%s", region, serviceName, resourceName)
			if e.metricsUsed[metricKey] == nil {
				e.metricsUsed[metricKey] = prometheus.NewDesc(
					"aws_service_limit_used_total",
					"Current used amount of the given AWS resource.",
					[]string{"region", "service", "resource"}, nil,
				)
			}
			if e.metricsLimit[metricKey] == nil {
				e.metricsLimit[metricKey] = prometheus.NewDesc(
					"aws_service_limit_limit_total",
					"Current limit of the given AWS resource.",
					[]string{"region", "service", "resource"}, nil,
				)
			}
			ch <- prometheus.MustNewConstMetric(
				e.metricsUsed[metricKey],
				prometheus.GaugeValue,
				used,
				region, serviceName, resourceName,
			)
			ch <- prometheus.MustNewConstMetric(
				e.metricsLimit[metricKey],
				prometheus.GaugeValue,
				limit,
				region, serviceName, resourceName,
			)
		}
	}
}

// parseFloat safely parses a string to float64, returns 0 on error
func parseFloat(s string) (float64, error) {
	s = strings.ReplaceAll(s, ",", "")
	return strconv.ParseFloat(s, 64)
}

// parseServiceNameFromCheck tries to extract a service name from the check metadata/title
func parseServiceNameFromCheck(result *support.TrustedAdvisorCheckResult) string {
	// You may want to improve this logic for your environment
	if result == nil || result.CheckId == nil {
		return "unknown"
	}
	// Example: use check ID as a proxy for service, or parse from result.Category
	return *result.CheckId
}
