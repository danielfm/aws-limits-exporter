package main

import (
	"flag"
	"net/http"

	"github.com/danielfm/aws-limits-exporter/core"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	VERSION = "UNKNOWN"
	addr    = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
	region  = flag.String("region", "", "The AWS region to show metrics for (default all regions).")
)

func main() {
	flag.Parse()

	glog.Infof("AWS Limits Exporter v%s started.", VERSION)

	exporter := core.NewSupportExporter(*region)
	go exporter.SupportClient.RequestServiceLimitsRefreshLoop()

	prometheus.MustRegister(exporter)

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	glog.Infof("Server listening on %s", *addr)
	glog.Fatal(http.ListenAndServe(*addr, nil))
}
