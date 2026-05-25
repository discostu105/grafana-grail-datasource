package plugin

import (
	"strings"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

func TestMapSeverity(t *testing.T) {
	cases := map[string]string{
		"INFO":          "info",
		"info":          "info",
		"err":           "error",
		"ERROR":         "error",
		"warning":       "warning",
		"warn":          "warning",
		"CRITICAL":      "critical",
		"fatal":         "critical",
		"debug":         "debug",
		"trace":         "trace",
		"informational": "info",
		"notice":        "info",
		"emerg":         "critical",
		"":              "unknown",
		"weird":         "unknown",
	}
	for in, want := range cases {
		if got := mapSeverity(in); got != want {
			t.Errorf("mapSeverity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPickKey(t *testing.T) {
	rec := map[string]interface{}{"content": "hi", "body": "fallback"}
	if k := pickKey(rec, "", "content", "body"); k != "content" {
		t.Errorf("pickKey first-match: got %q", k)
	}
	if k := pickKey(rec, "body", "content"); k != "body" {
		t.Errorf("pickKey preferred should win: got %q", k)
	}
	if k := pickKey(rec, "missing", "nope", "alsonope"); k != "" {
		t.Errorf("pickKey no-match should return empty: got %q", k)
	}
}

func TestParseRecordTime(t *testing.T) {
	rec := map[string]interface{}{
		"ts_rfc":   "2026-05-25T10:00:00Z",
		"ts_nano":  "2026-05-25T10:00:00.123456789Z",
		"ts_ms":    float64(1_700_000_000_000),
		"ts_s":     float64(1_700_000_000),
		"ts_bogus": "not a time",
	}
	cases := map[string]bool{
		"ts_rfc":  true,
		"ts_nano": true,
		"ts_ms":   true,
		"ts_s":    true,
	}
	for k := range cases {
		got := parseRecordTime(rec, k)
		if got.IsZero() {
			t.Errorf("parseRecordTime(%q) = zero", k)
		}
	}
	// Bogus string still returns *something* (now()), never zero.
	if got := parseRecordTime(rec, "ts_bogus"); got.IsZero() {
		t.Errorf("bogus value should default to now, not zero")
	}
	// Empty key returns now.
	if got := parseRecordTime(rec, ""); got.IsZero() {
		t.Errorf("empty key should default to now, not zero")
	}
}

func TestRecordsToLogFrame_Shape(t *testing.T) {
	records := []map[string]interface{}{
		{
			"timestamp":  "2026-05-25T10:00:00Z",
			"content":    "Hello from loxone",
			"loglevel":   "INFO",
			"host.name":  "lox",
			"control.name": "Heizung",
		},
		{
			"timestamp": "2026-05-25T10:00:01Z",
			"content":   "Error happened",
			"loglevel":  "ERROR",
			"host.name": "lox",
		},
	}
	frames, err := recordsToLogFrame("A", records, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("want 1 frame, got %d", len(frames))
	}
	f := frames[0]
	if f.Meta == nil || f.Meta.PreferredVisualization != data.VisTypeLogs {
		t.Errorf("frame should have logs vis hint, got %+v", f.Meta)
	}
	names := make([]string, 0, len(f.Fields))
	for _, fld := range f.Fields {
		names = append(names, fld.Name)
	}
	expected := []string{"time", "body", "level", "labels"}
	if strings.Join(names, ",") != strings.Join(expected, ",") {
		t.Errorf("field names = %v, want %v", names, expected)
	}
	// Severity mapping: INFO → info, ERROR → error
	levelField := f.Fields[2]
	if levelField.At(0) != "info" || levelField.At(1) != "error" {
		t.Errorf("level mapping wrong: %v / %v", levelField.At(0), levelField.At(1))
	}
}

func TestRecordsToLogFrame_Empty(t *testing.T) {
	frames, err := recordsToLogFrame("A", nil, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("expected one (empty) frame, got %d", len(frames))
	}
	if frames[0].Meta == nil || frames[0].Meta.PreferredVisualization != data.VisTypeLogs {
		t.Errorf("empty logs frame should still carry the logs vis hint")
	}
}

func TestJSONLabels_ExcludesReservedColumns(t *testing.T) {
	rec := map[string]interface{}{
		"body":      "should not appear",
		"timestamp": "should not appear",
		"level":     "should not appear",
		"host":      "lox",
		"count":     float64(42),
		"missing":   nil,
	}
	out := jsonLabels(rec, "body", "timestamp", "level")
	if strings.Contains(out, "body") || strings.Contains(out, "timestamp") || strings.Contains(out, "level") {
		t.Errorf("reserved columns leaked: %q", out)
	}
	if !strings.Contains(out, `"host":"lox"`) {
		t.Errorf("missing host label: %q", out)
	}
	if !strings.Contains(out, `"count":"42"`) {
		t.Errorf("missing count label: %q", out)
	}
	if strings.Contains(out, "missing") {
		t.Errorf("nil labels should be skipped: %q", out)
	}
}

// Sanity: the level field at row N matches what mapSeverity would emit
// for the same input. Guards against a regression in the mapping table.
func TestLogsFrameLevelMapping_Roundtrip(t *testing.T) {
	for _, sev := range []string{"INFO", "ERROR", "WARN", "DEBUG", "TRACE", "CRITICAL", "weird"} {
		records := []map[string]interface{}{{
			"timestamp": time.Now().Format(time.RFC3339),
			"content":   "x",
			"loglevel":  sev,
		}}
		frames, _ := recordsToLogFrame("A", records, "")
		got := frames[0].Fields[2].At(0)
		want := mapSeverity(sev)
		if got != want {
			t.Errorf("severity %q: got %q, want %q", sev, got, want)
		}
	}
}
