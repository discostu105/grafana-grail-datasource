package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/discostu105/dynatracegrail/pkg/dynatrace"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

// queryTimeout is the per-query context deadline, covering execute + poll.
const queryTimeout = 60 * time.Second

type Datasource struct {
	dt     *dynatrace.Client
	cfgErr error
}

type instanceJSON struct {
	TenantURL string `json:"tenantUrl"`
}

func NewDatasource(_ context.Context, s backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	var cfg instanceJSON
	if len(s.JSONData) > 0 {
		if err := json.Unmarshal(s.JSONData, &cfg); err != nil {
			return &Datasource{cfgErr: fmt.Errorf("parsing jsonData: %w", err)}, nil
		}
	}
	token := s.DecryptedSecureJSONData["apiToken"]
	c, err := dynatrace.New(cfg.TenantURL, token)
	if err != nil {
		return &Datasource{cfgErr: err}, nil
	}
	return &Datasource{dt: c}, nil
}

func (d *Datasource) Dispose() {}

type queryModel struct {
	DqlQuery string `json:"dqlQuery"`
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

	cctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	from, to := q.TimeRange.From, q.TimeRange.To
	dql := substituteMacros(qm.DqlQuery, from, to)

	dqlResp, err := d.dt.Query(cctx, dql, from, to)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	records := dqlResp.GetRecords()
	logRawShape(q.RefID, dql, records)

	frames, err := recordsToFrames(q.RefID, records)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("mapping records: %v", err))
	}
	return backend.DataResponse{Frames: frames}
}

func (d *Datasource) CheckHealth(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	if d.cfgErr != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: d.cfgErr.Error()}, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := d.dt.Query(cctx, "data record(x = 1)", time.Time{}, time.Time{}); err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: err.Error()}, nil
	}
	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: "DQL OK"}, nil
}

// substituteMacros replaces Grafana's $__timeFrom / $__timeTo macros in the
// DQL string with the panel's actual range as RFC3339 timestamps. Anything
// else (e.g. $__interval) is left untouched — Dynatrace DQL has its own
// time-bucketing syntax.
func substituteMacros(dql string, from, to time.Time) string {
	r := strings.NewReplacer(
		"$__timeFrom", from.UTC().Format(time.RFC3339),
		"$__timeTo", to.UTC().Format(time.RFC3339),
		"${__timeFrom}", from.UTC().Format(time.RFC3339),
		"${__timeTo}", to.UTC().Format(time.RFC3339),
	)
	return r.Replace(dql)
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
