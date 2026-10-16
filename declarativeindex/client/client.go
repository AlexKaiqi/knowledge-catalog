package client

import (
	"context"
	"fmt"
	"kc/internal/httpjson"
	"net/url"
	"strings"
)

type client struct {
	baseURL   string
	transport Transport
}

func New(config Config) (Client, error) {
	base := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("declarative-index client base URL must be an http(s) URL without credentials, query, or fragment")
	}
	if config.Transport == nil {
		return nil, fmt.Errorf("declarative-index client requires an authenticated transport")
	}
	return &client{baseURL: base, transport: config.Transport}, nil
}
func (c *client) request(ctx context.Context, path string, input any, options RequestOptions, out any) error {
	return httpjson.Do(ctx, c.transport, c.baseURL, "POST", path, input, options.RequestID, out)
}
func (c *client) DescribeProjection(ctx context.Context, r ProjectionRequest, o RequestOptions, out any) error {
	return c.request(ctx, "/operations/v1/projections:describe", r, o, out)
}
func (c *client) SyncProjection(ctx context.Context, r ProjectionRequest, o RequestOptions, out any) error {
	return c.request(ctx, "/operations/v1/projections:sync", r, o, out)
}
