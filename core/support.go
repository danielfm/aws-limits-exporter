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
		if check.Id != nil && check.Category != nil && *check.Category == "service_limits" {
			ids = append(ids, *check.Id)
		}
	}
	return ids, nil
}

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
				glog.Warningf("No result for check: %s", checkID)
				continue
			}

			// Defensive nil-checks for summary fields
			status := ""
			if result.Status != nil {
				status = *result.Status
			}
			flagged := int64(0)
			processed := int64(0)
			if result.ResourcesSummary != nil {
				if result.ResourcesSummary.ResourcesFlagged != nil {
					flagged = *result.ResourcesSummary.ResourcesFlagged
				}
				if result.ResourcesSummary.ResourcesProcessed != nil {
					processed = *result.ResourcesSummary.ResourcesProcessed
				}
			}

			glog.Infof("Check '%s' summary: Status: %s, FlaggedResources: %d, ResourcesProcessed: %d",
				checkID, status, flagged, processed,
			)

			if result.FlaggedResources == nil {
				glog.Warningf("No FlaggedResources for check: %s", checkID)
				continue
			}
			for i, res := range result.FlaggedResources {
				regionVal := "<nil>"
				if res.Region != nil {
					regionVal = *res.Region
				}
				statusVal := "<nil>"
				if res.Status != nil {
					statusVal = *res.Status
				}
				var meta []string
				if res.Metadata != nil && len(res.Metadata) > 0 {
					for _, m := range res.Metadata {
						if m != nil {
							meta = append(meta, *m)
						} else {
							meta = append(meta, "<nil>")
						}
					}
				} else {
					meta = append(meta, "<empty>")
				}
				glog.Infof("  Resource[%d]: Region=%s | Status=%s | Metadata=%v",
					i,
					regionVal,
					statusVal,
					meta,
				)
				if i == 4 && len(result.FlaggedResources) > 5 {
					glog.Infof("...only showing first 5 flagged resources")
					break
				}
			}
		}
		glog.Infof("Waiting %d minutes until the next refresh...", waitMs/60000)
		time.Sleep(time.Duration(waitMs) * time.Millisecond)
	}
}

func NewSupportExporter(region string) *SupportExporter {
	return &SupportExporter{
		SupportClient: NewSupportClient(region),
		metricsRegion: region,
		metricsUsed:   make(map[string]*prometheus.Desc),
		metricsLimit:  make(map[string]*prometheus.Desc),
	}
}

func (e *SupportExporter) Describe(ch chan<- *prometheus.Desc) {}

func (e *SupportExporter) Collect(ch chan<- prometheus.Metric) {
	checkIDs, err := e.SupportClient.GetAvailableCheckIDs()
	if err != nil {
		glog.Errorf("Failed to get available Trusted Advisor checks: %v", err)
		return
	}
	for _, checkID := range checkIDs {
		result, err := e.SupportClient.DescribeServiceLimitsCheckResult(checkID)
		if err != nil {
			glog.Errorf("Cannot retrieve Trusted Advisor check results data: %v", err)
			continue
		}
		if result == nil || result.FlaggedResources == nil {
			glog.Warningf("No flagged resources for check: %s", checkID)
			continue
		}
		for _, res := range result.FlaggedResources {
			if res.Metadata == nil || len(res.Metadata) < 3 {
				glog.Warningf("Resource metadata too short for check %s: %v", checkID, res.Metadata)
				continue
			}

			// Dynamically find used and limit fields: the last two numeric fields in metadata
			limitIdx := -1
			usedIdx := -1
			for i := len(res.Metadata) - 1; i >= 0; i-- {
				if res.Metadata[i] == nil {
					continue
				}
				_, err := strconv.ParseFloat(strings.ReplaceAll(*res.Metadata[i], ",", ""), 64)
				if err == nil {
					if limitIdx == -1 {
						limitIdx = i
					} else if usedIdx == -1 {
						usedIdx = i
						break
					}
				}
			}
			if usedIdx == -1 || limitIdx == -1 || usedIdx >= limitIdx {
				glog.Warningf("Resource metadata for check %s does not contain parseable used/limit fields: %v", checkID, res.Metadata)
				continue
			}

			resourceName := "-"
			if len(res.Metadata) > 0 && res.Metadata[0] != nil {
				resourceName = *res.Metadata[0]
			}
			usedStr := *res.Metadata[usedIdx]
			limitStr := *res.Metadata[limitIdx]
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

func parseFloat(s string) (float64, error) {
	s = strings.ReplaceAll(s, ",", "")
	return strconv.ParseFloat(s, 64)
}

func parseServiceNameFromCheck(result *support.TrustedAdvisorCheckResult) string {
	if result == nil || result.CheckId == nil {
		return "unknown"
	}
	return *result.CheckId
}
