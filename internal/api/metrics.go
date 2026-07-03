package api

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	runsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bifrost_pipeline_runs_total",
		Help: "Total pipeline runs by final status.",
	}, []string{"status"})

	runDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "bifrost_pipeline_run_duration_seconds",
		Help:    "Pipeline run duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"status"})

	runningRuns = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "bifrost_running_runs",
		Help: "Number of pipeline runs currently executing.",
	})
)
