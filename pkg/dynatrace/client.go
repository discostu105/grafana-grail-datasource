// Package dynatrace wraps the dtctl SDK with the narrow surface this plugin
// needs: client construction from explicit credentials and DQL execution.
package dynatrace

import (
	"context"
	"fmt"
	"time"

	"github.com/dynatrace-oss/dtctl/sdk/api/query"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

type Client struct {
	handler *query.Handler
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

	return &Client{handler: query.NewHandler(httpClient)}, nil
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
