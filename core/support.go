package core

import (
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/support"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

// Service limits check
// Auto Scaling - Groups		fW7HH0l7J9
// Auto Scaling - Launch Configurations		aW7HH0l7J9
// CloudFormation - Stacks		gW7HH0l7J9
// DynamoDB Read Capacity		6gtQddfEw6
// DynamoDB Write Capacity 	c5ftjdfkMr
// EBS - Active Snapshots		eI7KK0l7J9
// EBS - Active Volumes		fH7LL0l7J9
// EBS - Cold HDD (sc1) 	gH5CC0e3J9
// EBS - General Purpose SSD Volume Storage		dH7RR0l6J9
// EBS - Magnetic (standard) Volume Storage		cG7HH0l7J9
// EBS Throughput Optimized HDD (st1)		wH7DD0l3J9
// EBS - Provisioned IOPS (SSD) Volume Aggregate IOPS		tV7YY0l7J9
// EBS - Provisioned IOPS SSD (io1) Volume Storage		gI7MM0l7J9
// EC2 - Elastic IP Addresses		aW9HH0l8J6
// EC2 - On-Demand Instances		0Xc6LMYG8P
// EC2 - Reserved Instance Leases		iH7PP0l7J9
// ELB - Active Load Balancers		iK7OO0l7J9
// IAM - Group		sU7XX0l7J9
// IAM - Instance Profiles		nO7SS0l7J9
// IAM - Policies		pR7UU0l7J9
// IAM - Roles		oQ7TT0l7J9
// IAM - Server Certificates		rT7WW0l7J9
// IAM - Users		qS7VV0l7J9
// Kinesis - Shards per Region		bW7HH0l7J9
// RDS - Cluster Parameter Groups		jtlIMO3qZM
// RDS - Cluster roles		7fuccf1Mx7
// RDS - Clusters		gjqMBn6pjz
// RDS - DB Instances		XG0aXHpIEt
// RDS - DB Parameter Groups		jEECYg2YVU
// RDS - DB Security Groups		gfZAn3W7wl
// RDS - DB snapshots per user		dV84wpqRUs
// RDS - Event Subscriptions		keAhfbH5yb
// RDS - Max Auths per Security Group		dBkuNCvqn5
// RDS - Option Groups		3Njm0DJQO9
// RDS - Read Replicas per Master		pYW8UkYz2w
// RDS - Reserved Instances		UUDvOa5r34
// RDS - Subnet Groups		dYWBaXaaMM
// RDS - Subnets per Subnet Group		jEhCtdJKOY
// RDS - Total Storage Quota		P1jhKWEmLa
// Route 53 Hosted Zones		dx3xfcdfMr
// Route 53 Max Health Checks		ru4xfcdfMr
// Route 53 Reusable Delegation Sets		ty3xfcdfMr
// Route 53 Traffic Policies		dx3xfbjfMr
// Route 53 Traffic Policy Instances		dx8afcdfMr
// SES - Daily Sending Quota		hJ7NN0l7J9
// VPC - Elastic IP Address		lN7RR0l7J9
// VPC - Internet Gateways		kM7QQ0l7J9
// VPC - Network Interfaces		jL7PP0l7J9
var (
	checkIDs = []string{
		"fW7HH0l7J9",
		"aW7HH0l7J9",
		"gW7HH0l7J9",
		"6gtQddfEw6",
		"c5ftjdfkMr",
		"eI7KK0l7J9",
		"fH7LL0l7J9",
		"gH5CC0e3J9",
		"dH7RR0l6J9",
		"cG7HH0l7J9",
		"wH7DD0l3J9",
		"tV7YY0l7J9",
		"gI7MM0l7J9",
		"aW9HH0l8J6",
		"0Xc6LMYG8P",
		"iH7PP0l7J9",
		"iK7OO0l7J9",
		"sU7XX0l7J9",
		"nO7SS0l7J9",
		"pR7UU0l7J9",
		"oQ7TT0l7J9",
		"rT7WW0l7J9",
		"qS7VV0l7J9",
		"bW7HH0l7J9",
		"jtlIMO3qZM",
		"7fuccf1Mx7",
		"gjqMBn6pjz",
		"XG0aXHpIEt",
		"jEECYg2YVU",
		"gfZAn3W7wl",
		"dV84wpqRUs",
		"keAhfbH5yb",
		"dBkuNCvqn5",
		"3Njm0DJQO9",
		"pYW8UkYz2w",
		"UUDvOa5r34",
		"dYWBaXaaMM",
		"jEhCtdJKOY",
		"P1jhKWEmLa",
		"dx3xfcdfMr",
		"ru4xfcdfMr",
		"ty3xfcdfMr",
		"dx3xfbjfMr",
		"dx8afcdfMr",
		"hJ7NN0l7J9",
		"lN7RR0l7J9",
		"kM7QQ0l7J9",
		"jL7PP0l7J9",
	}
)

// validateRegionName ...
func validateRegionName(region string) {
	if region != "" {
		availableRegions := endpoints.AwsPartition().Regions()
		if _, ok := availableRegions[region]; !ok {
			regions := make([]string, 0, len(availableRegions))
			for key := range availableRegions {
				regions = append(regions, key)
			}

			glog.Fatalf("Invalid AWS region %s, valid regions: %s", region, strings.Join(regions, ","))
		}
	}
}

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
	var (
		waitMs int64 = 3600000
	)

	for {
		for _, checkID := range checkIDs {
			input := &support.RefreshTrustedAdvisorCheckInput{
				CheckId: aws.String(checkID),
			}
			glog.Infof("Refreshing Trusted Advisor check '%s'...", checkID)
			_, err := client.SupportClient.RefreshTrustedAdvisorCheck(input)
			if err != nil {
				glog.Errorf("Error when requesting status refresh: %v", err)
				continue
			}
		}
		glog.Infof("Waiting %d minutes until the next refresh...", waitMs/60000)
		time.Sleep(time.Duration(waitMs) * time.Millisecond)
	}

}

// DescribeServiceLimitsCheckResult ...
func (client *SupportClientImpl) DescribeServiceLimitsCheckResult(checkID string) (*support.TrustedAdvisorCheckResult, error) {
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
func NewSupportExporter(region string) *SupportExporter {
	validateRegionName(region)

	client := NewSupportClient()

	return &SupportExporter{
		supportClient: client,
		metricsRegion: region,
		metricsUsed:   map[string]*prometheus.Desc{},
		metricsLimit:  map[string]*prometheus.Desc{},
	}
}

// validateMetricRegion
func (e *SupportExporter) validateMetricRegion(region string) bool {
	if e.metricsRegion == "" {
		return true
	} else if e.metricsRegion == region {
		return true
	}
	return false
}

// RequestServiceLimitsRefreshLoop ...
func (e *SupportExporter) RequestServiceLimitsRefreshLoop() {
	e.supportClient.RequestServiceLimitsRefreshLoop()
}

// Describe ...
func (e *SupportExporter) Describe(ch chan<- *prometheus.Desc) {
	for _, checkID := range checkIDs {
		result, err := e.supportClient.DescribeServiceLimitsCheckResult(checkID)
		if err != nil {
			glog.Errorf("Cannot retrieve Trusted Advisor check results data: %v", err)
		}

		for _, resource := range result.FlaggedResources {
			resourceID := aws.StringValue(resource.ResourceId)

			// Sanity check in order not to report the same metric more than once
			if _, ok := e.metricsUsed[resourceID]; ok {
				continue
			}

			region := aws.StringValue(resource.Metadata[0])

			// Filter region
			if !e.validateMetricRegion(region) {
				continue
			}

			serviceName := aws.StringValue(resource.Metadata[1])
			serviceNameLower := strings.ToLower(serviceName)

			e.metricsUsed[resourceID] = newServerMetric(region, serviceNameLower, "used_total", "Current used amount of the given resource.", []string{"resource"})
			e.metricsLimit[resourceID] = newServerMetric(region, serviceNameLower, "limit_total", "Current limit of the given resource.", []string{"resource"})

			ch <- e.metricsUsed[resourceID]
			ch <- e.metricsLimit[resourceID]
		}
	}
}

// Collect ...
func (e *SupportExporter) Collect(ch chan<- prometheus.Metric) {
	for _, checkID := range checkIDs {
		result, err := e.supportClient.DescribeServiceLimitsCheckResult(checkID)
		if err != nil {
			glog.Errorf("Cannot retrieve Trusted Advisor check results data: %v", err)
			continue
		}

		for _, resource := range result.FlaggedResources {
			resourceID := aws.StringValue(resource.ResourceId)

			// Sanity check in order not to report the same metric more than once
			metricUsed, ok := e.metricsUsed[resourceID]
			if !ok {
				continue
			}

			// Filter region
			if !e.validateMetricRegion(aws.StringValue(resource.Metadata[0])) {
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
}

func newServerMetric(region, subSystem, metricName, docString string, labels []string) *prometheus.Desc {
	return prometheus.NewDesc(
		prometheus.BuildFQName("aws", subSystem, metricName),
		docString, labels, prometheus.Labels{"region": region},
	)
}
