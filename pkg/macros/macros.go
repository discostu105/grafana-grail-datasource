// Package macros expands Grafana-style $__macros in a DQL string against a
// time range. Expansion runs server-side so the alerting / annotation paths
// (which have no frontend templateSrv) get the same substitutions as the
// regular panel path.
//
// Supported macros:
//
//	$__timeFrom            ISO-8601 of TimeRange.From (RFC3339)
//	$__timeTo              ISO-8601 of TimeRange.To
//	$__fromTime            alias for $__timeFrom
//	$__toTime              alias for $__timeTo
//	$__from                from epoch ms (integer literal)
//	$__to                  to epoch ms (integer literal)
//	$__interval            DQL duration literal closest to the panel interval
//	                       — chosen from a fixed ladder {1s,5s,10s,30s,1m,5m,
//	                       10m,30m,1h,3h,6h,12h,1d}; used inside DQL after
//	                       interval: parameters.
//	$__interval_ms         integer milliseconds
//	$__timeFilter(<f>)     "<f> >= \"<from>\" and <f> <= \"<to>\"" where the
//	                       timestamps are RFC3339 strings — matches the way
//	                       Grail filters event tables.
//	$__timeFilter()        same as above but uses "timestamp" as the field.
//
// `${name}` and `$name` are both accepted (Grafana uses both spellings).
// Expansion is idempotent: a second pass over an already-expanded string is
// a no-op since the substitutes don't reintroduce any `$__…` tokens.
package macros

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Range describes the time range that drives the substitutions.
type Range struct {
	From     time.Time
	To       time.Time
	Interval time.Duration // panel interval; 0 lets Expand pick from the range
}

// Expand replaces every recognised macro in dql with its evaluated string.
// Unknown $__ tokens are left untouched.
func Expand(dql string, r Range) string {
	out := expandTimeFilter(dql, r)

	// Replacements with fixed text — ordered longest-first so that
	// $__interval_ms is replaced before $__interval.
	replacements := []struct {
		from, to string
	}{
		{"$__interval_ms", intervalMs(r)},
		{"${__interval_ms}", intervalMs(r)},
		{"$__interval", intervalDQL(r)},
		{"${__interval}", intervalDQL(r)},
		{"$__timeFrom", rfc(r.From)},
		{"${__timeFrom}", rfc(r.From)},
		{"$__timeTo", rfc(r.To)},
		{"${__timeTo}", rfc(r.To)},
		{"$__fromTime", rfc(r.From)},
		{"${__fromTime}", rfc(r.From)},
		{"$__toTime", rfc(r.To)},
		{"${__toTime}", rfc(r.To)},
		{"$__from", fmt.Sprintf("%d", millis(r.From))},
		{"${__from}", fmt.Sprintf("%d", millis(r.From))},
		{"$__to", fmt.Sprintf("%d", millis(r.To))},
		{"${__to}", fmt.Sprintf("%d", millis(r.To))},
	}
	pairs := make([]string, 0, len(replacements)*2)
	for _, p := range replacements {
		pairs = append(pairs, p.from, p.to)
	}
	return strings.NewReplacer(pairs...).Replace(out)
}

// timeFilterRe matches both $__timeFilter(field) and $__timeFilter() — the
// argument can be any identifier path (letters, digits, dots, underscores).
var timeFilterRe = regexp.MustCompile(`\$\{?__timeFilter\}?\(\s*([A-Za-z0-9_.]*)\s*\)`)

func expandTimeFilter(dql string, r Range) string {
	return timeFilterRe.ReplaceAllStringFunc(dql, func(match string) string {
		sub := timeFilterRe.FindStringSubmatch(match)
		field := sub[1]
		if field == "" {
			field = "timestamp"
		}
		return fmt.Sprintf("%s >= %q and %s <= %q", field, rfc(r.From), field, rfc(r.To))
	})
}

func rfc(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func millis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func intervalMs(r Range) string {
	return fmt.Sprintf("%d", chooseInterval(r).Milliseconds())
}

func intervalDQL(r Range) string {
	d := chooseInterval(r)
	return dqlDuration(d)
}

// intervalLadder is what Grafana would expose via $__interval — a fixed set
// of "nice" buckets; we pick the smallest one that yields ≤ ~200 buckets
// across the range.
var intervalLadder = []time.Duration{
	time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second,
	time.Minute, 5 * time.Minute, 10 * time.Minute, 30 * time.Minute,
	time.Hour, 3 * time.Hour, 6 * time.Hour, 12 * time.Hour,
	24 * time.Hour,
}

func chooseInterval(r Range) time.Duration {
	if r.Interval > 0 {
		return snapInterval(r.Interval)
	}
	if r.From.IsZero() || r.To.IsZero() || !r.To.After(r.From) {
		return time.Minute
	}
	target := r.To.Sub(r.From) / 200
	return snapInterval(target)
}

func snapInterval(target time.Duration) time.Duration {
	for _, d := range intervalLadder {
		if d >= target {
			return d
		}
	}
	return intervalLadder[len(intervalLadder)-1]
}

// dqlDuration formats a Go duration as a DQL duration literal (e.g. 5m, 1h).
func dqlDuration(d time.Duration) string {
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d%time.Second == 0:
		return fmt.Sprintf("%ds", int(d/time.Second))
	default:
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
}
