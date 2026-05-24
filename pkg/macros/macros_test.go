package macros

import (
	"strings"
	"testing"
	"time"
)

func TestExpand(t *testing.T) {
	from := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)
	r := Range{From: from, To: to}

	cases := []struct {
		in   string
		want string
	}{
		{"$__timeFrom", "2026-05-24T10:00:00Z"},
		{"${__timeFrom}", "2026-05-24T10:00:00Z"},
		{"$__timeTo", "2026-05-24T12:00:00Z"},
		{"$__fromTime", "2026-05-24T10:00:00Z"},
		{"$__toTime", "2026-05-24T12:00:00Z"},
		{"$__from", "1779616800000"},
		{"$__to", "1779624000000"},
		{"$__interval_ms", "60000"},
		{"$__interval", "1m"},
		{`$__timeFilter(timestamp)`, `timestamp >= "2026-05-24T10:00:00Z" and timestamp <= "2026-05-24T12:00:00Z"`},
		{`$__timeFilter()`, `timestamp >= "2026-05-24T10:00:00Z" and timestamp <= "2026-05-24T12:00:00Z"`},
		{`$__timeFilter(start_time)`, `start_time >= "2026-05-24T10:00:00Z" and start_time <= "2026-05-24T12:00:00Z"`},
		{`fetch x | filter $__timeFilter(dt.timestamp)`, `fetch x | filter dt.timestamp >= "2026-05-24T10:00:00Z" and dt.timestamp <= "2026-05-24T12:00:00Z"`},
	}
	for _, c := range cases {
		got := Expand(c.in, r)
		if got != c.want {
			t.Errorf("Expand(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExpandIdempotent(t *testing.T) {
	r := Range{
		From: time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 5, 24, 6, 0, 0, 0, time.UTC),
	}
	in := `fetch logs | filter $__timeFilter() | summarize count(), by:{bin($__interval, timestamp)}`
	once := Expand(in, r)
	twice := Expand(once, r)
	if once != twice {
		t.Errorf("not idempotent:\n once: %q\n twice: %q", once, twice)
	}
	if strings.Contains(once, "$__") {
		t.Errorf("expanded form still contains $__: %q", once)
	}
}

func TestSnapInterval(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want time.Duration
	}{
		{500 * time.Millisecond, time.Second},
		{3 * time.Second, 5 * time.Second},
		{45 * time.Second, time.Minute},
		{15 * time.Minute, 30 * time.Minute},
		{2 * time.Hour, 3 * time.Hour},
		{72 * time.Hour, 24 * time.Hour},
	}
	for _, c := range cases {
		got := snapInterval(c.in)
		if got != c.want {
			t.Errorf("snapInterval(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestChooseInterval_AutoTarget(t *testing.T) {
	// 200 buckets over 24h → 432s target → snaps to 10m
	r := Range{
		From: time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
	}
	got := chooseInterval(r)
	if got != 10*time.Minute {
		t.Errorf("chooseInterval over 24h = %v, want 10m", got)
	}
}

func TestExpand_UnknownMacroLeftAlone(t *testing.T) {
	r := Range{
		From: time.Now(),
		To:   time.Now().Add(time.Hour),
	}
	in := "fetch x | filter foo == $__notARealMacro"
	out := Expand(in, r)
	if !strings.Contains(out, "$__notARealMacro") {
		t.Errorf("unknown macro should be left alone, got %q", out)
	}
}
