package plugin

import "testing"

func TestApplyAdhocFilters_Empty(t *testing.T) {
	in := "fetch logs | limit 10"
	got := applyAdhocFilters(in, nil)
	if got != in {
		t.Errorf("no filters should not change DQL: %q -> %q", in, got)
	}
}

func TestApplyAdhocFilters_EmptyClearsMacro(t *testing.T) {
	got := applyAdhocFilters("fetch logs | filter $__adhocFilters", nil)
	want := "fetch logs | filter true"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyAdhocFilters_AutoAppend(t *testing.T) {
	got := applyAdhocFilters("fetch logs | limit 10",
		[]AdhocFilter{{Key: "host.name", Operator: "=", Value: "h1"}})
	want := "fetch logs | limit 10\n| filter host.name == \"h1\""
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyAdhocFilters_MacroSubstitution(t *testing.T) {
	got := applyAdhocFilters(
		`fetch logs | filter $__adhocFilters and timestamp > now() - 1h`,
		[]AdhocFilter{
			{Key: "host.name", Operator: "=", Value: "h1"},
			{Key: "service.name", Operator: "!=", Value: "prod"},
		},
	)
	want := `fetch logs | filter host.name == "h1" AND service.name != "prod" and timestamp > now() - 1h`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyAdhocFilters_DropsEmptyKeyOrValue(t *testing.T) {
	got := applyAdhocFilters("fetch logs",
		[]AdhocFilter{
			{Key: "", Operator: "=", Value: "h1"},
			{Key: "host.name", Operator: "=", Value: ""},
			{Key: "host.name", Operator: "=", Value: "h1"},
		})
	want := "fetch logs\n| filter host.name == \"h1\""
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyAdhocFilters_RegexOperators(t *testing.T) {
	got := applyAdhocFilters("fetch x",
		[]AdhocFilter{
			{Key: "host.name", Operator: "=~", Value: "h.*"},
			{Key: "host.name", Operator: "!~", Value: "tmp.*"},
		})
	want := `fetch x` + "\n" + `| filter matchesRegex(host.name, "h.*") AND not matchesRegex(host.name, "tmp.*")`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
