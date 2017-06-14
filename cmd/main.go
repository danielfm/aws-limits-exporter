package main

import (
	"flag"
	"net/http"

	"github.com/golang/glog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/danielfm/aws-limits-exporter/core"
)

// VERSION set by build script
var VERSION = "UNKNOWN"

var addr = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
var region = flag.String("region", "us-east-1", "Returns usage and limits for the given AWS Region.")

func main() {
	flag.Parse()

	glog.Infof("AWS Limits Exporter v%s started.", VERSION)

	exporter := core.NewSupportExporter(*region)
	go exporter.RequestServiceLimitsRefreshLoop()

	prometheus.Register(exporter)

	http.Handle("/metrics", promhttp.Handler())
	glog.Fatal(http.ListenAndServe(*addr, nil))
}
