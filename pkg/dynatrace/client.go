// Package dynatrace wraps the dtctl SDK with the narrow surface this plugin
// needs: client construction from explicit credentials and DQL execution.
package dynatrace

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/sdk/api/query"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

type Client struct {
	handler   *query.Handler
	tenantURL string
	token     string
	http      *http.Client
}

// New constructs an authenticated DQL client from a tenant URL and platform
// token.
func New(tenantURL, token string) (*Client, error) {
	if tenantURL == "" {
		return nil, fmt.Errorf("tenant URL is empty")
	}
	if token == "" {
		return nil, fmt.Errorf("API token is empty")
	}

	httpClient, err := httpclient.New(tenantURL, httpclient.WithToken(token))
	if err != nil {
		return nil, fmt.Errorf("constructing http client: %w", err)
	}

	return &Client{
		handler:   query.NewHandler(httpClient),
		tenantURL: strings.TrimRight(tenantURL, "/"),
		token:     token,
		http:      &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// Query runs a DQL query via execute+poll. Zero from/to means "let Grail use
// its defaults" (used by CheckHealth's `data record` probe). Non-zero values
// are passed as DefaultTimeframeStart/End and only apply when the DQL itself
// does not specify a from:/to: clause.
func (c *Client) Query(ctx context.Context, dql string, from, to time.Time) (*query.Response, error) {
	req := query.ExecuteRequest{Query: dql}
	if !from.IsZero() {
		req.DefaultTimeframeStart = from.UTC().Format(time.RFC3339)
	}
	if !to.IsZero() {
		req.DefaultTimeframeEnd = to.UTC().Format(time.RFC3339)
	}
	return c.handler.ExecuteAndPoll(ctx, req, nil)
}

// Autocomplete proxies Grail's autocomplete endpoint at
// /platform/storage/query/v1/query:autocomplete. body is the raw JSON
// payload (e.g. `{"query":"fetch ","position":6}`). The response body is
// streamed back verbatim — the Grafana plugin's resource handler returns it
// to the frontend completion provider directly.
func (c *Client) Autocomplete(ctx context.Context, body []byte) ([]byte, error) {
	endpoint := c.tenantURL + "/platform/storage/query/v1/query:autocomplete"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("autocomplete: HTTP %d: %s", resp.StatusCode, string(out))
	}
	return out, nil
}
