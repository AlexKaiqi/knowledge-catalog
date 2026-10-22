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
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("object client base URL must be an http(s) URL without credentials, query, or fragment")
	}
	if config.Transport == nil {
		return nil, fmt.Errorf("object client requires an authenticated transport")
	}
	return &client{baseURL: baseURL, transport: config.Transport}, nil
}

func (c *client) request(ctx context.Context, method, path string, input any, options RequestOptions, output any) error {
	return httpjson.Do(ctx, c.transport, c.baseURL, method, path, input, options.RequestID, output)
}

func (c *client) Read(ctx context.Context, r ReadRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/object/v1/objects:read", r, o, out)
}
func (c *client) Compare(ctx context.Context, r ComparisonRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/object/v1/objects:diff", r, o, out)
}
func (c *client) Resolve(ctx context.Context, r ResolveRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/object/v1/objects:resolve", r, o, out)
}
func (c *client) Log(ctx context.Context, r ObjectRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/object/v1/log:query", r, o, out)
}
func (c *client) Provenance(ctx context.Context, r ObjectRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/object/v1/provenance:describe", r, o, out)
}
func (c *client) Schema(ctx context.Context, r SchemaRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/object/v1/schemas:describe", r, o, out)
}
func (c *client) BrowseSchemas(ctx context.Context, r SchemaPageRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/object/v1/schemas:list", r, o, out)
}
func (c *client) Commit(ctx context.Context, repo string, r CommitRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/writer/v1/repositories/"+url.PathEscape(repo)+"/commits", r, o, out)
}
func (c *client) Put(ctx context.Context, r MaintenanceRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/object/v1/objects:put", r, o, out)
}
func (c *client) Remove(ctx context.Context, r MaintenanceRequest, o RequestOptions, out any) error {
	return c.request(ctx, "POST", "/object/v1/objects:remove", r, o, out)
}
func (c *client) Receipt(ctx context.Context, id string, o RequestOptions, out any) error {
	return c.request(ctx, "GET", "/writer/v1/receipts/"+url.PathEscape(id), nil, o, out)
}
