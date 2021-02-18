package scraper

import (
	"log"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Exporter exports stats in prometheus format
type Exporter struct {
	metrics Metrics
	entries []TargetResponse
}

// NewExporter creates a new exporter
func NewExporter(options Metrics, chSize int) *Exporter {
	return &Exporter{
		metrics: options,
		entries: make([]TargetResponse, 0, chSize),
	}
}

// Describe describe the metrics for prometheus
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {

	e.metrics.TargetURLStatus.Describe(ch)
	e.metrics.TargetURLResponseTime.Describe(ch)
}

// Collect collects data to be consumed by prometheus
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {

	for _, res := range e.entries {
		log.Println("collect: ", res)
		e.metrics.TargetURLStatus.
			WithLabelValues(res.URL.String()).
			Set(float64(res.Status))

		e.metrics.TargetURLResponseTime.
			WithLabelValues(res.URL.String()).
			Observe(float64(res.ResponseTime.Milliseconds()))
	}
	e.metrics.TargetURLStatus.Collect(ch)
	e.metrics.TargetURLResponseTime.Collect(ch)
}

// Metrics is a collection of the url metrics
type Metrics struct {
	TargetURLStatus       *prometheus.GaugeVec
	TargetURLResponseTime *prometheus.HistogramVec
}

// NewMetrics builds a new metric options
func NewMetrics() Metrics {
	us := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "sample",
			Subsystem: "external",
			Name:      "url_up",
			Help:      "URL status",
		},
		[]string{"url"},
	)

	uRH := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "sample",
			Subsystem: "external",
			Name:      "url_response_time_ms",
			Help:      "URL response time in milli seconds",
		},
		[]string{"url"},
	)

	metrics := Metrics{
		TargetURLStatus:       us,
		TargetURLResponseTime: uRH,
	}

	return metrics
}

// PrometheusHandler prometheus metrics handler
func PrometheusHandler() http.Handler {
	return promhttp.Handler()
}

// For syncronisation to make sure RegisterExporter is called only once
var once = sync.Once{}

// RegisterExporter registers the exporter with prometheus
func RegisterExporter(e *ScrapePool) {
	once.Do(func() {
		prometheus.MustRegister(e)
	})
}
