package plugin

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// recordsToFrames maps DQL records into Grafana data frames.
//
// Two shapes are supported:
//
//   - Timeseries: each record has a "timestamp array" column (RFC3339 strings)
//     plus one or more parallel value arrays. One frame is emitted per record
//     (= one series), with the timestamp as the time field and each value
//     array as a separate float field. Remaining scalar columns become labels.
//
//   - Table: records have only scalar columns (or none of them have a usable
//     timestamp array). All records are collapsed into a single frame with one
//     column per key, and each record becomes a row.
func recordsToFrames(refID string, records []map[string]interface{}) ([]*data.Frame, error) {
	if len(records) == 0 {
		return nil, nil
	}

	if isTimeseriesShape(records[0]) {
		frames := make([]*data.Frame, 0, len(records))
		for i, rec := range records {
			frame, err := recordToTimeseriesFrame(refID, rec)
			if err != nil {
				return frames, fmt.Errorf("record %d: %w", i, err)
			}
			frames = append(frames, frame)
		}
		return frames, nil
	}

	frame, err := recordsToTableFrame(refID, records)
	if err != nil {
		return nil, err
	}
	return []*data.Frame{frame}, nil
}

// isTimeseriesShape returns true if the record contains a column that parses
// as an array of RFC3339 timestamps.
func isTimeseriesShape(rec map[string]interface{}) bool {
	_, _, err := extractTimestamps(rec)
	return err == nil
}

func recordToTimeseriesFrame(refID string, rec map[string]interface{}) (*data.Frame, error) {
	tsKey, times, err := extractTimestamps(rec)
	if err != nil {
		return nil, err
	}

	keys := sortedKeys(rec, tsKey)
	labels := data.Labels{}
	valueKeys := make([]string, 0)
	valueArrs := make(map[string][]interface{}, 0)
	for _, k := range keys {
		switch v := rec[k].(type) {
		case []interface{}:
			if len(v) == len(times) {
				valueKeys = append(valueKeys, k)
				valueArrs[k] = v
			}
		case string:
			labels[k] = v
		case bool:
			labels[k] = fmt.Sprintf("%t", v)
		case float64:
			labels[k] = formatFloatLabel(v)
		case nil:
			// skip
		default:
			// objects/maps left out of labels
		}
	}

	frame := data.NewFrame(refID,
		data.NewField("time", nil, times),
	)
	for _, k := range valueKeys {
		vals := toFloat64Slice(valueArrs[k])
		frame.Fields = append(frame.Fields, data.NewField(k, labels, vals))
	}
	return frame, nil
}

// recordsToTableFrame produces a single tabular frame: one row per record,
// one column per key (union across all records). Column types are inferred
// from the first non-nil value seen for each key.
func recordsToTableFrame(refID string, records []map[string]interface{}) (*data.Frame, error) {
	keys := unionKeys(records)
	colKinds := make(map[string]colKind, len(keys))
	for _, k := range keys {
		colKinds[k] = inferColKind(records, k)
	}

	frame := data.NewFrame(refID)
	for _, k := range keys {
		field := newFieldForKind(k, colKinds[k], len(records))
		for i, rec := range records {
			setFieldCell(field, i, rec[k], colKinds[k])
		}
		frame.Fields = append(frame.Fields, field)
	}
	return frame, nil
}

type colKind int

const (
	kindString colKind = iota
	kindFloat
	kindBool
	kindTime
)

func inferColKind(records []map[string]interface{}, key string) colKind {
	for _, rec := range records {
		switch v := rec[key].(type) {
		case nil:
			continue
		case float64:
			return kindFloat
		case bool:
			return kindBool
		case string:
			if _, err := time.Parse(time.RFC3339Nano, v); err == nil {
				return kindTime
			}
			if _, err := time.Parse(time.RFC3339, v); err == nil {
				return kindTime
			}
			return kindString
		default:
			return kindString
		}
	}
	return kindString
}

func newFieldForKind(name string, kind colKind, n int) *data.Field {
	switch kind {
	case kindFloat:
		return data.NewField(name, nil, make([]*float64, n))
	case kindBool:
		return data.NewField(name, nil, make([]*bool, n))
	case kindTime:
		return data.NewField(name, nil, make([]*time.Time, n))
	default:
		return data.NewField(name, nil, make([]*string, n))
	}
}

func setFieldCell(f *data.Field, row int, raw interface{}, kind colKind) {
	switch kind {
	case kindFloat:
		switch n := raw.(type) {
		case float64:
			v := n
			f.Set(row, &v)
		case nil:
			f.Set(row, (*float64)(nil))
		default:
			nan := math.NaN()
			f.Set(row, &nan)
		}
	case kindBool:
		switch b := raw.(type) {
		case bool:
			v := b
			f.Set(row, &v)
		case nil:
			f.Set(row, (*bool)(nil))
		}
	case kindTime:
		if s, ok := raw.(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				f.Set(row, &t)
				return
			}
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				f.Set(row, &t)
				return
			}
		}
		f.Set(row, (*time.Time)(nil))
	default:
		s := stringifyCell(raw)
		if raw == nil {
			f.Set(row, (*string)(nil))
		} else {
			f.Set(row, &s)
		}
	}
}

func stringifyCell(v interface{}) string {
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
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprintf("%v", x)
		}
		return string(b)
	}
}

func unionKeys(records []map[string]interface{}) []string {
	set := map[string]struct{}{}
	for _, r := range records {
		for k := range r {
			set[k] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// extractTimestamps finds the column holding the per-bucket timestamps.
// Strategy: prefer a key literally named "timestamp"; otherwise pick the
// first []interface{} whose elements parse as RFC3339 strings.
func extractTimestamps(rec map[string]interface{}) (string, []time.Time, error) {
	if raw, ok := rec["timestamp"]; ok {
		if arr, ok := raw.([]interface{}); ok {
			if ts, err := parseTimestampArray(arr); err == nil {
				return "timestamp", ts, nil
			}
		}
	}
	for _, k := range sortedKeys(rec, "") {
		raw, ok := rec[k].([]interface{})
		if !ok || len(raw) == 0 {
			continue
		}
		if _, ok := raw[0].(string); !ok {
			continue
		}
		if ts, err := parseTimestampArray(raw); err == nil {
			return k, ts, nil
		}
	}
	return "", nil, fmt.Errorf("no timestamp array column found in record")
}

func parseTimestampArray(arr []interface{}) ([]time.Time, error) {
	out := make([]time.Time, len(arr))
	for i, v := range arr {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("element %d is %T, want string", i, v)
		}
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t, err = time.Parse(time.RFC3339, s)
			if err != nil {
				return nil, fmt.Errorf("element %d: %w", i, err)
			}
		}
		out[i] = t
	}
	return out, nil
}

// toFloat64Slice converts a []interface{} of JSON numbers (and nils) into a
// dense []*float64. nils become a nil pointer; numbers wrap into a pointer so
// Grafana can render gaps correctly.
func toFloat64Slice(arr []interface{}) []*float64 {
	out := make([]*float64, len(arr))
	for i, v := range arr {
		switch n := v.(type) {
		case float64:
			f := n
			out[i] = &f
		case nil:
			out[i] = nil
		default:
			nan := math.NaN()
			out[i] = &nan
		}
	}
	return out
}

func sortedKeys(rec map[string]interface{}, exclude string) []string {
	keys := make([]string, 0, len(rec))
	for k := range rec {
		if k == exclude {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatFloatLabel(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}
