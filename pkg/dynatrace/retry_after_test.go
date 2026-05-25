package dynatrace

import (
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"   ", 0},
		{"5", 5 * time.Second},
		{"0", 0},
		// Negative or unparseable falls back to 0
		{"-1", 0},
		{"not a number", 0},
	}
	for _, c := range cases {
		got := parseRetryAfter(c.in)
		if got != c.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRetryAfterError_ErrorString(t *testing.T) {
	e := &retryAfterError{StatusCode: 429, Body: "rate limited"}
	if got := e.Error(); got != "HTTP 429: rate limited" {
		t.Errorf("retryAfterError.Error() = %q", got)
	}
}
