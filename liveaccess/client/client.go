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
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || config.Transport == nil {
		return nil, fmt.Errorf("LiveAccess client requires an HTTP server origin and authenticated transport")
	}
	return &client{baseURL: base, transport: config.Transport}, nil
}

func (c *client) ResolveBinding(ctx context.Context, request BindingRequest, options RequestOptions, output any) error {
	return httpjson.Do(ctx, c.transport, c.baseURL, "POST", "/knowledge/v1/bindings:resolve", request, options.RequestID, output)
}

func (c *client) AccessResource(ctx context.Context, request AccessRequest, options RequestOptions, output any) error {
	return httpjson.Do(ctx, c.transport, c.baseURL, "POST", "/knowledge/v1/resources:access", request, options.RequestID, output)
}
