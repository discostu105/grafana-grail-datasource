package plugin

import (
	"fmt"
	"strings"
)

// AdhocFilter mirrors what Grafana attaches to every panel query when the
// dashboard has ad-hoc filters from the top bar.
type AdhocFilter struct {
	Key      string `json:"key"`
	Operator string `json:"operator"` // "=", "!=", "=~", "!~"
	Value    string `json:"value"`
}

// applyAdhocFilters substitutes the $__adhocFilters macro with the AND-joined
// filter expression. If the macro isn't present in the DQL and there are
// filters to apply, append `| filter <expr>` to the pipeline so the user
// gets filtering even when their DQL doesn't reference the macro.
//
// Empty filters → no change. Filter values are shell-escaped with %q so
// embedded quotes/backslashes survive.
func applyAdhocFilters(dql string, filters []AdhocFilter) string {
	filters = normalizeFilters(filters)
	if len(filters) == 0 {
		// Still need to clear the macro if it's there but empty.
		return strings.NewReplacer("$__adhocFilters", "true", "${__adhocFilters}", "true").Replace(dql)
	}
	expr := buildFilterExpr(filters)
	if strings.Contains(dql, "$__adhocFilters") || strings.Contains(dql, "${__adhocFilters}") {
		return strings.NewReplacer("$__adhocFilters", expr, "${__adhocFilters}", expr).Replace(dql)
	}
	// Auto-append. Strip trailing whitespace so we don't end up with double
	// separators when the user already ended the line with a newline.
	return strings.TrimRight(dql, " \n\t") + "\n| filter " + expr
}

// normalizeFilters drops filters with empty key OR empty value (Grafana
// emits these as placeholders while the user is typing).
func normalizeFilters(in []AdhocFilter) []AdhocFilter {
	out := make([]AdhocFilter, 0, len(in))
	for _, f := range in {
		if strings.TrimSpace(f.Key) == "" || strings.TrimSpace(f.Value) == "" {
			continue
		}
		out = append(out, f)
	}
	return out
}

func buildFilterExpr(filters []AdhocFilter) string {
	parts := make([]string, 0, len(filters))
	for _, f := range filters {
		parts = append(parts, formatOne(f))
	}
	return strings.Join(parts, " AND ")
}

// formatOne returns one filter as a DQL expression. Defaults to ==.
// Regex operators map to matchesRegex().
func formatOne(f AdhocFilter) string {
	switch f.Operator {
	case "!=":
		return fmt.Sprintf("%s != %q", f.Key, f.Value)
	case "=~":
		return fmt.Sprintf("matchesRegex(%s, %q)", f.Key, f.Value)
	case "!~":
		return fmt.Sprintf("not matchesRegex(%s, %q)", f.Key, f.Value)
	default: // "=" or anything else
		return fmt.Sprintf("%s == %q", f.Key, f.Value)
	}
}
