package scraper

import (
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	model "github.com/prometheus/client_model/go"
)

type metricResult struct {
	value  float64
	labels map[string]string
}

type metricResultHistogram struct {
	sampleSum   float64
	sampleCount uint64
	labels      map[string]string
}

func labels2Map(labels []*model.LabelPair) map[string]string {
	res := map[string]string{}
	for _, l := range labels {
		res[l.GetName()] = l.GetValue()
	}
	return res
}

func readGauge(g prometheus.Metric) metricResult {
	m := &model.Metric{}
	g.Write(m)

	return metricResult{
		value:  m.GetGauge().GetValue(),
		labels: labels2Map(m.GetLabel()),
	}
}

func readHistogram(g prometheus.Metric) metricResultHistogram {
	m := &model.Metric{}
	g.Write(m)

	return metricResultHistogram{
		sampleSum:   *m.GetHistogram().SampleSum,
		sampleCount: *m.GetHistogram().SampleCount,
		labels:      labels2Map(m.GetLabel()),
	}
}

func Test_Desribe(t *testing.T) {
	metrics := NewMetrics()
	exporter := NewExporter(metrics, 1)

	ch := make(chan *prometheus.Desc)

	go exporter.Describe(ch)

	d := <-ch

	expectedExternalServiceUpDesc := `Desc{fqName: "sample_external_url_up", help: "URL status", constLabels: {}, variableLabels: [url]}`
	actualExternalServiceUpDesc := d.String()
	if expectedExternalServiceUpDesc != actualExternalServiceUpDesc {
		t.Errorf("Want: %s, got: %s", expectedExternalServiceUpDesc, actualExternalServiceUpDesc)
	}

	d = <-ch
	expectedExternalServiceResponseTimeMS := `Desc{fqName: "sample_external_url_response_time_ms", help: "URL response time in milli seconds", constLabels: {}, variableLabels: [url]}`
	actualExternalServiceResponseTimeMS := d.String()
	if expectedExternalServiceResponseTimeMS != actualExternalServiceResponseTimeMS {
		t.Errorf("Want: %s, got: %s", expectedExternalServiceResponseTimeMS, actualExternalServiceResponseTimeMS)
	}

}

func Test_Collect(t *testing.T) {
	metrics := NewMetrics()
	exporter := NewExporter(metrics, 10)

	serverURL, _ := url.Parse("https://foo.com")
	expectedQueryResult := TargetResponse{
		URL:          serverURL,
		Status:       HealthGood,
		ResponseTime: time.Duration(2 * time.Second),
	}

	exporter.entries = []TargetResponse{expectedQueryResult}

	ch := make(chan prometheus.Metric, 10)

	defer close(ch)

	go exporter.Collect(ch)
	g := (<-ch).(prometheus.Gauge)
	result := readGauge(g)
	if expectedQueryResult.URL.String() != result.labels["url"] {
		t.Errorf("Want: %s, got: %s", expectedQueryResult.URL, result.labels["url"])
	}

	if float64(expectedQueryResult.Status) != result.value {
		t.Errorf("Want: %f, got: %f", float64(expectedQueryResult.Status), result.value)
	}

	g1 := (<-ch).(prometheus.Histogram)
	hResult := readHistogram(g1)

	if expectedQueryResult.URL.String() != hResult.labels["url"] {
		t.Errorf("Want: %s, got: %s", expectedQueryResult.URL, hResult.labels["url"])
	}

	if float64(expectedQueryResult.ResponseTime.Milliseconds()) != hResult.sampleSum {
		t.Errorf("Want: %f, got: %f", float64(expectedQueryResult.ResponseTime.Milliseconds()), hResult.sampleSum)
	}

	if 1 != hResult.sampleCount {
		t.Errorf("Want: %d, got: %d", 1, hResult.sampleCount)
	}
}
