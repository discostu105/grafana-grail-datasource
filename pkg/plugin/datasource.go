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

	"github.com/discostu105/dynatracegrail/pkg/dynatrace"
	"github.com/discostu105/dynatracegrail/pkg/macros"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ backend.CallResourceHandler   = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

const (
	defaultQueryTimeout = 30 * time.Second
	defaultTimeframe    = time.Hour
)

type Datasource struct {
	dt           *dynatrace.Client
	cfg          settings
	cfgErr       error
	queryTimeout time.Duration
	defaultRange time.Duration
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

	if err := validateTenantURL(cfg.TenantURL); err != nil {
		ds.cfgErr = err
		return ds, nil
	}
	token := s.DecryptedSecureJSONData["apiToken"]
	if token == "" {
		ds.cfgErr = errors.New("API token is empty — set it in the data source config page")
		return ds, nil
	}

	c, err := dynatrace.New(cfg.TenantURL, token)
	if err != nil {
		ds.cfgErr = err
		return ds, nil
	}
	ds.dt = c
	return ds, nil
}

func validateTenantURL(raw string) error {
	if raw == "" {
		return errors.New("tenant URL is empty — set it in the data source config page")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("tenant URL must look like https://<env>.apps.dynatrace.com, got %q", raw)
	}
	if !strings.Contains(u.Host, ".dynatrace.com") {
		return fmt.Errorf("tenant URL host %q does not look like a Dynatrace endpoint", u.Host)
	}
	return nil
}

func (d *Datasource) Dispose() {}

// CallResource exposes plugin-side HTTP endpoints to the frontend. Available:
//
//	POST /autocomplete   { "query": "<DQL>", "position": <int> }
//	  → proxies to Grail's /platform/storage/query/v1/query:autocomplete
//	    and returns the raw response body. Used by the Monaco completion
//	    provider in QueryEditor.tsx.
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
			return sender.Send(&backend.CallResourceResponse{
				Status: http.StatusBadGateway,
				Body:   []byte(err.Error()),
			})
		}
		return sender.Send(&backend.CallResourceResponse{
			Status:  http.StatusOK,
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			Body:    body,
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
	QueryType string `json:"queryType"` // "timeseries" (default) | "logs"
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
	if d.cfgErr != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, d.cfgErr.Error())
	}
	var qm queryModel
	if err := json.Unmarshal(q.JSON, &qm); err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("unmarshal query: %v", err))
	}
	if qm.DqlQuery == "" {
		return backend.ErrDataResponse(backend.StatusBadRequest, "dqlQuery is empty")
	}

	cctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
	defer cancel()

	from, to := d.resolveTimeRange(q)
	interval := q.Interval
	dql := applyAdhocFilters(qm.DqlQuery, qm.AdhocFilters)
	dql = macros.Expand(dql, macros.Range{From: from, To: to, Interval: interval})

	start := time.Now()
	dqlResp, err := d.dt.Query(cctx, dql, from, to)
	if err != nil {
		log.DefaultLogger.Warn("dql query failed", "refID", q.RefID, "err", err, "duration", time.Since(start))
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	records := dqlResp.GetRecords()
	logRawShape(q.RefID, dql, records)

	var frames []*data.Frame
	if qm.QueryType == "logs" {
		frames, err = recordsToLogFrame(q.RefID, records, qm.LogBodyField)
	} else {
		frames, err = recordsToFrames(q.RefID, records)
	}
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("mapping records: %v", err))
	}
	applyLegendFormat(frames, qm.LegendFormat)
	inferDecimals(frames)
	log.DefaultLogger.Info("dql query ok", "refID", q.RefID, "queryType", qm.QueryType, "rows", len(records), "frames", len(frames), "duration", time.Since(start))
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
//   - DQL execute returned a Grail error — body surfaced
func (d *Datasource) CheckHealth(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	if d.cfgErr != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: d.cfgErr.Error()}, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if _, err := d.dt.Query(cctx, "data record(x = 1)", time.Time{}, time.Time{}); err != nil {
		host := "tenant"
		if u, perr := url.Parse(d.cfg.TenantURL); perr == nil {
			host = u.Host
		}
		msg := classifyHealthError(host, err)
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: msg}, nil
	}
	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: fmt.Sprintf("Successfully connected to %s", hostOf(d.cfg.TenantURL))}, nil
}

func classifyHealthError(host string, err error) string {
	es := err.Error()
	switch {
	case strings.Contains(es, "no such host"), strings.Contains(es, "dial"), strings.Contains(es, "connection refused"):
		return fmt.Sprintf("Cannot reach %s: %v", host, err)
	case strings.Contains(es, "x509"), strings.Contains(es, "tls"):
		return fmt.Sprintf("TLS error talking to %s: %v", host, err)
	case strings.Contains(es, "401"), strings.Contains(es, "Unauthorized"), strings.Contains(es, "authentication"):
		return fmt.Sprintf("Authentication rejected by %s: %v", host, err)
	case strings.Contains(es, "403"), strings.Contains(es, "Forbidden"):
		return fmt.Sprintf("Token lacks required scopes (need storage:metrics:read or similar): %v", err)
	default:
		return fmt.Sprintf("DQL execute failed on %s: %v", host, err)
	}
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
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
