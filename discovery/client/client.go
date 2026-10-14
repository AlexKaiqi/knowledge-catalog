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
		return nil, fmt.Errorf("discovery client base URL must be an http(s) URL without credentials, query, or fragment")
	}
	if config.Transport == nil {
		return nil, fmt.Errorf("discovery client requires an authenticated transport")
	}
	return &client{baseURL: base, transport: config.Transport}, nil
}
func (c *client) request(ctx context.Context, method, path string, input any, options RequestOptions, output any) error {
	return httpjson.Do(ctx, c.transport, c.baseURL, method, path, input, options.RequestID, output)
}
func (c *client) Search(ctx context.Context, r SearchRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/object/v1/search", r, o, out)
}
func (c *client) Relations(ctx context.Context, r RelationsRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/object/v1/relations:query", r, o, out)
}
func (c *client) Traverse(ctx context.Context, r TraverseRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/object/v1/traverse:query", r, o, out)
}
func (c *client) DescribeAccessSpec(ctx context.Context, r AccessSpecDescribeRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/operations/v1/access-specs:describe", r, o, out)
}
func (c *client) PluginHelp(ctx context.Context, name string, o RequestOptions, out any) error {
	return c.request(ctx, "GET", "/object/v1/plugins/"+url.PathEscape(name), nil, o, out)
}
