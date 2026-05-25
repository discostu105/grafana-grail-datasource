package plugin

import (
	"fmt"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// recordsToLogFrame produces a single Grafana logs-flavored frame.
//
//   - PreferredVisualisation = VisTypeLogs (Grafana renders this as the logs
//     panel / Explore logs view).
//   - time field is the record's timestamp (string with `timestamp` /
//     `start_time` / `end_time` accepted, or the first parseable time).
//   - body field carries the log message — column name resolved as
//     bodyField, then `content` (DQL default), then `body`, then `message`.
//   - severity → level (the standard Grafana enum: critical / error /
//     warning / info / debug / trace). Mapped from `loglevel`, `severity`,
//     or `status`.
//   - All remaining columns become labels on each row via the conventional
//     Grafana `labels` JSON field (json-encoded map per row).
func recordsToLogFrame(refID string, records []map[string]interface{}, bodyField string) ([]*data.Frame, error) {
	if len(records) == 0 {
		f := data.NewFrame(refID)
		setLogVis(f)
		return []*data.Frame{f}, nil
	}

	bodyKey := pickKey(records[0], bodyField, "content", "body", "message", "msg")
	timeKey := pickKey(records[0], "", "timestamp", "@timestamp", "start_time", "time")
	levelKey := pickKey(records[0], "", "loglevel", "severity", "log.level", "status")

	times := make([]time.Time, len(records))
	bodies := make([]string, len(records))
	levels := make([]string, len(records))
	labelsField := make([]string, len(records))

	for i, rec := range records {
		times[i] = parseRecordTime(rec, timeKey)
		bodies[i] = stringField(rec, bodyKey)
		if levelKey != "" {
			levels[i] = mapSeverity(stringField(rec, levelKey))
		}
		labelsField[i] = jsonLabels(rec, bodyKey, timeKey, levelKey)
	}

	f := data.NewFrame(refID,
		data.NewField("time", nil, times),
		data.NewField("body", nil, bodies),
	)
	if levelKey != "" {
		f.Fields = append(f.Fields, data.NewField("level", nil, levels))
	}
	f.Fields = append(f.Fields, data.NewField("labels", nil, labelsField))
	setLogVis(f)
	return []*data.Frame{f}, nil
}

func setLogVis(f *data.Frame) {
	if f.Meta == nil {
		f.Meta = &data.FrameMeta{}
	}
	f.Meta.PreferredVisualization = data.VisTypeLogs
}

// pickKey returns the first key present in rec from a preference list. The
// preferred value (if non-empty) is checked first.
func pickKey(rec map[string]interface{}, preferred string, fallbacks ...string) string {
	if preferred != "" {
		if _, ok := rec[preferred]; ok {
			return preferred
		}
	}
	for _, k := range fallbacks {
		if _, ok := rec[k]; ok {
			return k
		}
	}
	return ""
}

func stringField(rec map[string]interface{}, key string) string {
	if key == "" {
		return ""
	}
	v := rec[key]
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func parseRecordTime(rec map[string]interface{}, key string) time.Time {
	if key == "" {
		return time.Now()
	}
	v := rec[key]
	switch x := v.(type) {
	case string:
		if t, err := time.Parse(time.RFC3339Nano, x); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, x); err == nil {
			return t
		}
	case float64:
		// epoch ms vs epoch s heuristic
		if x > 1e12 {
			return time.UnixMilli(int64(x))
		}
		return time.Unix(int64(x), 0)
	}
	return time.Now()
}

// mapSeverity normalises common log-level strings to Grafana's enum.
func mapSeverity(s string) string {
	switch strings.ToLower(s) {
	case "crit", "critical", "fatal", "emerg", "emergency":
		return "critical"
	case "err", "error":
		return "error"
	case "warn", "warning":
		return "warning"
	case "notice", "info", "informational":
		return "info"
	case "debug":
		return "debug"
	case "trace":
		return "trace"
	}
	return "unknown"
}

// jsonLabels JSON-encodes the row's non-reserved scalar columns into a
// labels object — what Grafana expects for the Explore logs panel's
// expandable per-row attribute list.
func jsonLabels(rec map[string]interface{}, body, ts, level string) string {
	exclude := map[string]bool{body: true, ts: true, level: true}
	parts := make([]string, 0, len(rec))
	for k, v := range rec {
		if exclude[k] {
			continue
		}
		s := scalarForLabel(v)
		if s == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%q:%q", k, s))
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func scalarForLabel(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return formatFloatLabel(x)
	case bool:
		return fmt.Sprintf("%t", x)
	default:
		return ""
	}
}
