package plugin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// CollectMetrics is invoked over gRPC by the Grafana server when the
// /api/admin/plugins/<id>/metrics admin endpoint is scraped. We serve the
// global Prometheus default registry (where promauto registered our
// counters + histograms below, plus the standard go_/process_ collectors).
func (d *Datasource) CollectMetrics(_ context.Context, _ *backend.CollectMetricsRequest) (*backend.CollectMetricsResult, error) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{}).ServeHTTP(rec, req)
	body := bytes.TrimRight(rec.Body.Bytes(), "\n")
	return &backend.CollectMetricsResult{PrometheusMetrics: body}, nil
}

// Custom counters + histograms registered on the global Prometheus
// registry. The Grafana plugin SDK exposes that registry at
// /api/plugins/<id>/metrics, alongside the standard go_/process_ series.
//
// No custom CollectMetricsHandler is needed — promauto + the SDK's
// default handler is enough. Earlier versions used a separate registry
// + manual ServeHTTP; v1.10.0 dropped that in favour of the simpler
// promauto path so the metrics actually reach the scrape endpoint.
var (
	queryRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "grafana_dql",
			Name:      "query_requests_total",
			Help:      "Total number of DQL queries executed, labelled by query type and status.",
		},
		[]string{"query_type", "status"},
	)
	queryDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "grafana_dql",
			Name:      "query_duration_seconds",
			Help:      "End-to-end DQL query duration, including Grail execute+poll and frame mapping.",
			Buckets:   prometheus.ExponentialBuckets(0.05, 2, 10), // 50ms .. ~25s
		},
		[]string{"query_type"},
	)
	autocompleteRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "grafana_dql",
			Name:      "autocomplete_requests_total",
			Help:      "Total number of /resources/autocomplete proxy calls, labelled by status.",
		},
		[]string{"status"},
	)
	dataObjectsRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "grafana_dql",
			Name:      "data_objects_requests_total",
			Help:      "Total number of /resources/data-objects lookups, labelled by status.",
		},
		[]string{"status"},
	)
)

// observeQuery records one query execution. status is one of "ok", "error",
// "bad_request"; queryType comes from the queryModel (logs / timeseries).
func observeQuery(queryType, status string, durationSeconds float64) {
	if queryType == "" {
		queryType = "timeseries"
	}
	queryRequestsTotal.WithLabelValues(queryType, status).Inc()
	queryDurationSeconds.WithLabelValues(queryType).Observe(durationSeconds)
}

// observeAutocomplete records one /resources/autocomplete proxy call.
func observeAutocomplete(status string) {
	autocompleteRequestsTotal.WithLabelValues(status).Inc()
}

// observeDataObjects records one /resources/data-objects lookup. Misses go
// through to Grail; hits are served from the in-memory cache and never
// increment this counter.
func observeDataObjects(status string) {
	dataObjectsRequestsTotal.WithLabelValues(status).Inc()
}
