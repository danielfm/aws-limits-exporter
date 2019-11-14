package main

import (
	"flag"
	"net/http"

	"github.com/golang/glog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/danielfm/aws-limits-exporter/core"
)

var (
	// VERSION set by build script
	VERSION = "UNKNOWN"

	addr = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
)

func main() {
	flag.Parse()

	glog.Infof("AWS Limits Exporter v%s started.", VERSION)

	exporter := core.NewSupportExporter()
	go exporter.RequestServiceLimitsRefreshLoop()

	prometheus.Register(exporter)

	http.Handle("/metrics", promhttp.Handler())
	glog.Fatal(http.ListenAndServe(*addr, nil))
}
