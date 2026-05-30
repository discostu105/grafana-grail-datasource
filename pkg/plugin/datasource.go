package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/discostu105/grail/pkg/dynatrace"
	"github.com/discostu105/grail/pkg/macros"
	dtquery "github.com/dynatrace-oss/dtctl/sdk/api/query"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ backend.CallResourceHandler   = (*Datasource)(nil)
	_ backend.CollectMetricsHandler = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

const (
	defaultQueryTimeout = 30 * time.Second
	defaultTimeframe    = time.Hour
	// pluginVersion is mirrored from package.json / plugin.json. Bump both
	// together on release; surfaced in the User-Agent so Dynatrace can
	// correlate plugin traffic.
	pluginVersion = "1.12.2"
	userAgent     = "discostu105-grail-datasource/" + pluginVersion
	// healthProbeQuery is a syntactically minimal DQL string used by
	// CheckHealth's Verify probe. It exercises auth + network without
	// consuming any Grail scan budget.
	healthProbeQuery = "fetch logs | limit 1"
)

type Datasource struct {
	dt           *dynatrace.Client
	cfg          settings
	cfgErr       error
	queryTimeout time.Duration
	defaultRange time.Duration
	dataObjects  dataObjectsCache
}

type settings struct {
	TenantURL           string `json:"tenantUrl"`
	QueryTimeoutSeconds int    `json:"queryTimeoutSeconds"`
	DefaultTimeframe    string `json:"defaultTimeframe"`
}

func NewDatasource(_ context.Context, s backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	var cfg settings
	if len(s.JSONData) > 0 {
		if err := json.Unmarshal(s.JSONData, &cfg); err != nil {
			return &Datasource{cfgErr: fmt.Errorf("parsing jsonData: %w", err)}, nil
		}
	}

	ds := &Datasource{
		cfg:          cfg,
		queryTimeout: defaultQueryTimeout,
		defaultRange: defaultTimeframe,
	}
	if cfg.QueryTimeoutSeconds > 0 {
		ds.queryTimeout = time.Duration(cfg.QueryTimeoutSeconds) * time.Second
	}
	if cfg.DefaultTimeframe != "" {
		if d, err := time.ParseDuration(cfg.DefaultTimeframe); err == nil {
			ds.defaultRange = d
		}
	}

	if err := dynatrace.ValidateTenantURL(cfg.TenantURL); err != nil {
		ds.cfgErr = err
		return ds, nil
	}
	token := s.DecryptedSecureJSONData["apiToken"]
	if err := dynatrace.ValidateToken(token); err != nil {
		ds.cfgErr = err
		return ds, nil
	}

	c, err := dynatrace.NewWith(cfg.TenantURL, token, dynatrace.Options{
		UserAgent: userAgent,
		Logger:    sdkLogger{},
	})
	if err != nil {
		ds.cfgErr = err
		return ds, nil
	}
	ds.dt = c
	return ds, nil
}

func (d *Datasource) Dispose() {}

// CallResource exposes plugin-side HTTP endpoints to the frontend. Available:
//
//	POST /autocomplete   { "query": "<DQL>", "position": <int> }
//	  → proxies to Grail's /platform/storage/query/v1/query:autocomplete
//	    and returns the raw response body. Used by the Monaco completion
//	    provider in QueryEditor.tsx.
//	GET  /data-objects
//	  → enumerates fetchable Grail tables via
//	    `fetch dt.system.data_objects | filter type == "table"`.
//	    Cached for an hour. Populates the Visual Query Builder's source dropdown.
func (d *Datasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	if d.cfgErr != nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusBadRequest,
			Body:   []byte(d.cfgErr.Error()),
		})
	}
	switch req.Path {
	case "autocomplete":
		body, err := d.dt.Autocomplete(ctx, req.Body)
		if err != nil {
			log.DefaultLogger.Warn("autocomplete proxy failed", "err", err)
			observeAutocomplete("error")
			return sender.Send(&backend.CallResourceResponse{
				Status: http.StatusBadGateway,
				Body:   []byte(err.Error()),
			})
		}
		observeAutocomplete("ok")
		return sender.Send(&backend.CallResourceResponse{
			Status:  http.StatusOK,
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			Body:    body,
		})
	case "data-objects":
		objs, err := d.listDataObjects(ctx)
		if err != nil {
			log.DefaultLogger.Warn("data-objects lookup failed", "err", err)
			observeDataObjects("error")
			return sender.Send(&backend.CallResourceResponse{
				Status: http.StatusBadGateway,
				Body:   []byte(err.Error()),
			})
		}
		observeDataObjects("ok")
		return sender.Send(&backend.CallResourceResponse{
			Status:  http.StatusOK,
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			Body:    marshalDataObjects(objs),
		})
	default:
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusNotFound,
			Body:   []byte("unknown resource: " + req.Path),
		})
	}
}

type queryModel struct {
	DqlQuery  string `json:"dqlQuery"`
	QueryType string `json:"queryType"` // "timeseries" (default) | "logs" | "traces"
	// LogBodyField names the column that carries the log message body when
	// QueryType == "logs". Defaults to "content" (the column DQL `fetch
	// logs` projects by default).
	LogBodyField string `json:"logBodyField,omitempty"`
	// LegendFormat is a Grafana display-name template applied to value
	// fields, e.g. "{{ control.name }} (avg)". Grafana resolves
	// ${__field.labels.X} at render time.
	LegendFormat string `json:"legendFormat,omitempty"`
	// AdhocFilters propagates the dashboard's top-bar filters (stamped on
	// by applyTemplateVariables on the frontend). Substituted into
	// $__adhocFilters or auto-appended as `| filter ...`.
	AdhocFilters []AdhocFilter `json:"adhocFilters,omitempty"`
}

func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	resp := backend.NewQueryDataResponse()
	for _, q := range req.Queries {
		resp.Responses[q.RefID] = d.query(ctx, q)
	}
	return resp, nil
}

func (d *Datasource) query(ctx context.Context, q backend.DataQuery) backend.DataResponse {
	start := time.Now()
	if d.cfgErr != nil {
		observeQuery("", "bad_request", time.Since(start).Seconds())
		return backend.ErrDataResponse(backend.StatusBadRequest, d.cfgErr.Error())
	}
	var qm queryModel
	if err := json.Unmarshal(q.JSON, &qm); err != nil {
		observeQuery("", "bad_request", time.Since(start).Seconds())
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("unmarshal query: %v", err))
	}
	if qm.DqlQuery == "" {
		observeQuery(qm.QueryType, "bad_request", time.Since(start).Seconds())
		return backend.ErrDataResponse(backend.StatusBadRequest, "dqlQuery is empty")
	}

	cctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
	defer cancel()

	from, to := d.resolveTimeRange(q)
	interval := q.Interval
	dql := applyAdhocFilters(qm.DqlQuery, qm.AdhocFilters)
	dql = macros.Expand(dql, macros.Range{From: from, To: to, Interval: interval})

	dqlResp, err := d.dt.Query(cctx, dql, from, to)
	if err != nil {
		log.DefaultLogger.Warn("dql query failed", "refID", q.RefID, "err", err, "duration", time.Since(start))
		observeQuery(qm.QueryType, "error", time.Since(start).Seconds())
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	records := dqlResp.GetRecords()
	logRawShape(q.RefID, dql, records)

	var frames []*data.Frame
	switch qm.QueryType {
	case "logs":
		frames, err = recordsToLogFrame(q.RefID, records, qm.LogBodyField)
	case "traces":
		// If the result carries span.id columns it's trace-detail; otherwise
		// it's a trace-list / aggregate over spans and the regular table
		// mapper renders it cleanly.
		if isTraceDetailShape(records) {
			frames, err = recordsToTraceFrame(q.RefID, records)
		} else {
			frames, err = recordsToFrames(q.RefID, records)
		}
	default:
		frames, err = recordsToFrames(q.RefID, records)
	}
	if err != nil {
		observeQuery(qm.QueryType, "error", time.Since(start).Seconds())
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("mapping records: %v", err))
	}
	applyLegendFormat(frames, qm.LegendFormat)
	inferDecimals(frames)
	inferMinMax(frames)
	attachNotifications(frames, dqlResp.GetNotifications())
	log.DefaultLogger.Info("dql query ok", "refID", q.RefID, "queryType", qm.QueryType, "rows", len(records), "frames", len(frames), "duration", time.Since(start))
	observeQuery(qm.QueryType, "ok", time.Since(start).Seconds())
	return backend.DataResponse{Frames: frames}
}

// resolveTimeRange picks the From/To passed to Grail. If the panel supplied a
// range, use it. Otherwise fall back to (now-defaultRange, now).
func (d *Datasource) resolveTimeRange(q backend.DataQuery) (time.Time, time.Time) {
	from, to := q.TimeRange.From, q.TimeRange.To
	if from.IsZero() || to.IsZero() || !to.After(from) {
		to = time.Now()
		from = to.Add(-d.defaultRange)
	}
	return from, to
}

// CheckHealth reports four distinct failure modes:
//   - tenant URL missing/malformed
//   - token missing
//   - HTTP transport failure (DNS, TLS, connection refused) — host included
//   - DQL Verify rejected by the API — body surfaced
func (d *Datasource) CheckHealth(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	if d.cfgErr != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: d.cfgErr.Error()}, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if _, err := d.dt.Verify(cctx, healthProbeQuery); err != nil {
		host := hostOf(d.cfg.TenantURL)
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: classifyHealthError(host, err)}, nil
	}
	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: fmt.Sprintf("Successfully connected to %s", hostOf(d.cfg.TenantURL))}, nil
}

// classifyHealthError turns a Verify failure into a user-actionable message.
// Prefers typed httpclient sentinels for status-based classification; falls
// back to substring matching on transport errors (DNS, TLS) which are not
// represented as typed errors by the SDK.
func classifyHealthError(host string, err error) string {
	switch {
	case errors.Is(err, httpclient.ErrUnauthorized):
		return fmt.Sprintf("Authentication rejected by %s: %v", host, err)
	case errors.Is(err, httpclient.ErrForbidden):
		return fmt.Sprintf("Token lacks required scopes (need storage:logs:read / storage:metrics:read or similar): %v", err)
	case errors.Is(err, httpclient.ErrRateLimited):
		return fmt.Sprintf("Rate limited by %s: %v", host, err)
	case errors.Is(err, httpclient.ErrServerError):
		return fmt.Sprintf("Server error from %s: %v", host, err)
	}

	es := err.Error()
	switch {
	case strings.Contains(es, "no such host"), strings.Contains(es, "dial"), strings.Contains(es, "connection refused"):
		return fmt.Sprintf("Cannot reach %s: %v", host, err)
	case strings.Contains(es, "x509"), strings.Contains(es, "tls"):
		return fmt.Sprintf("TLS error talking to %s: %v", host, err)
	default:
		return fmt.Sprintf("DQL verify failed on %s: %v", host, err)
	}
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

// attachNotifications surfaces Grail notifications (sampling notes, scan-limit
// warnings, deprecations) as Grafana notices on the first frame so panels can
// display them in the inspector. Attaching to one frame avoids per-series
// duplication in multi-series responses.
func attachNotifications(frames []*data.Frame, notifications []dtquery.Notification) {
	if len(frames) == 0 || len(notifications) == 0 {
		return
	}
	notices := make([]data.Notice, 0, len(notifications))
	for _, n := range notifications {
		text := n.Message
		if text == "" {
			continue
		}
		notices = append(notices, data.Notice{
			Severity: noticeSeverity(n.Severity),
			Text:     text,
		})
	}
	if len(notices) == 0 {
		return
	}
	frames[0].AppendNotices(notices...)
}

func noticeSeverity(s string) data.NoticeSeverity {
	switch strings.ToUpper(s) {
	case "WARN", "WARNING":
		return data.NoticeSeverityWarning
	case "ERROR", "SEVERE":
		return data.NoticeSeverityError
	default:
		return data.NoticeSeverityInfo
	}
}

// sdkLogger adapts dtctl's httpclient.Logger interface onto Grafana's
// backend logger. Used for retry / connection diagnostics.
type sdkLogger struct{}

func (sdkLogger) Debugf(format string, args ...interface{}) {
	log.DefaultLogger.Debug(fmt.Sprintf(format, args...))
}

// logRawShape emits the key set and per-key value types for the first record,
// at debug level. Lets us iterate on frame mapping against real responses.
func logRawShape(refID, dql string, records []map[string]interface{}) {
	if len(records) == 0 {
		log.DefaultLogger.Debug("dql records: empty", "refID", refID, "dql", dql)
		return
	}
	first := records[0]
	keys := make(map[string]string, len(first))
	for k, v := range first {
		switch a := v.(type) {
		case []interface{}:
			if len(a) > 0 {
				keys[k] = fmt.Sprintf("[]%T (len=%d, first=%v)", a[0], len(a), a[0])
			} else {
				keys[k] = "[] (empty)"
			}
		default:
			keys[k] = fmt.Sprintf("%T (%v)", v, v)
		}
	}
	log.DefaultLogger.Debug("dql records: first row shape",
		"refID", refID, "dql", dql, "rowCount", len(records), "columns", keys)
}
