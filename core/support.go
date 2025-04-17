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

// Create a new AWS Support client for the given region
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

// Returns all Trusted Advisor check IDs that are in the 'service_limits' category
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

// Returns the result for an individual Trusted Advisor check
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

// Loops forever, actively refreshing all TA service_limits checks for the account/region
func (client *SupportClientImpl) RequestServiceLimitsRefreshLoop() {
	var waitMs int64 = 3600000 // 1 hour between refreshes
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
				// Collect all metadata, replacing nils for logging
				var meta []string
				if res.Metadata != nil && len(res.Metadata) > 0 {
					for idx, m := range res.Metadata {
						if m != nil {
							meta = append(meta, fmt.Sprintf("[%d]=%q", idx, *m))
						} else {
							meta = append(meta, fmt.Sprintf("[%d]=<nil>", idx))
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

// Constructs the Prometheus exporter
func NewSupportExporter(region string) *SupportExporter {
	return &SupportExporter{
		SupportClient: NewSupportClient(region),
		metricsRegion: region,
		metricsUsed:   make(map[string]*prometheus.Desc),
		metricsLimit:  make(map[string]*prometheus.Desc),
	}
}

// Required Prometheus interface (dynamic metrics: nothing to describe here)
func (e *SupportExporter) Describe(ch chan<- *prometheus.Desc) {}

// Main Prometheus collector for metrics
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

			// --- Find the two numeric fields at the end of metadata (limit and used)
			limitIdx := -1
			usedIdx := -1
			found := 0
			for i := len(res.Metadata) - 1; i >= 0; i-- {
				if res.Metadata[i] == nil {
					continue
				}
				_, err := strconv.ParseFloat(strings.ReplaceAll(*res.Metadata[i], ",", ""), 64)
				if err == nil {
					if found == 0 {
						usedIdx = i
					} else if found == 1 {
						limitIdx = i
						break
					}
					found++
				}
			}
			if usedIdx == -1 || limitIdx == -1 || limitIdx >= usedIdx {
				// Log the actual metadata values for human readability
				var metaStrings []string
				for idx, m := range res.Metadata {
					if m != nil {
						metaStrings = append(metaStrings, fmt.Sprintf("[%d]=%q", idx, *m))
					} else {
						metaStrings = append(metaStrings, fmt.Sprintf("[%d]=<nil>", idx))
					}
				}
				glog.Infof("Skipping resource for check %s: no parseable used/limit fields: %s", checkID, strings.Join(metaStrings, ", "))
				continue
			}

			// --- Extract labels for Prometheus
			regionLabel := "-"
			exportedServiceLabel := "-"
			resourceLabel := "-"
			if len(res.Metadata) > 2 {
				if res.Metadata[0] != nil {
					regionLabel = *res.Metadata[0]
				}
				if res.Metadata[1] != nil {
					exportedServiceLabel = *res.Metadata[1]
				}
				if res.Metadata[2] != nil {
					resourceLabel = *res.Metadata[2]
				}
			}
			// Treat global (non-region) resources as region="global"
			if regionLabel == "-" {
				regionLabel = "global"
			}
			// If used or limit field is null, treat as zero per AWS/Prometheus best practice
			usedStr := "0"
			if res.Metadata[usedIdx] != nil {
				usedStr = *res.Metadata[usedIdx]
			}
			limitStr := "0"
			if res.Metadata[limitIdx] != nil {
				limitStr = *res.Metadata[limitIdx]
			}
			used, err1 := parseFloat(usedStr)
			limit, err2 := parseFloat(limitStr)
			if err1 != nil || err2 != nil {
				glog.Infof("Cannot parse used/limit for check %s, resource %s/%s/%s: %v/%v", checkID, regionLabel, exportedServiceLabel, resourceLabel, err1, err2)
				continue
			}

			// Avoid redundant Prometheus descriptors for each label set
			metricKey := fmt.Sprintf("%s_%s_%s", regionLabel, exportedServiceLabel, resourceLabel)
			if e.metricsUsed[metricKey] == nil {
				e.metricsUsed[metricKey] = prometheus.NewDesc(
					"aws_service_used",
					"Current used amount of the given AWS resource.",
					[]string{"region", "exported_service", "resource"}, nil,
				)
			}
			if e.metricsLimit[metricKey] == nil {
				e.metricsLimit[metricKey] = prometheus.NewDesc(
					"aws_service_limit",
					"Current limit of the given AWS resource.",
					[]string{"region", "exported_service", "resource"}, nil,
				)
			}
			// -- Emit metrics!
			ch <- prometheus.MustNewConstMetric(
				e.metricsUsed[metricKey],
				prometheus.GaugeValue,
				used,
				regionLabel, exportedServiceLabel, resourceLabel,
			)
			ch <- prometheus.MustNewConstMetric(
				e.metricsLimit[metricKey],
				prometheus.GaugeValue,
				limit,
				regionLabel, exportedServiceLabel, resourceLabel,
			)
		}
	}
}

// Helper: makes parsing numeric fields robust against commas and missing fields
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
