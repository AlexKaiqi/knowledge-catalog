package client

import (
	"context"
	"net/http"
	"strings"

	clientauth "kc/client/auth"
	declarativeindexclient "kc/declarativeindex/client"
	discoveryclient "kc/discovery/client"
	"kc/internal/httpjson"
	accessclient "kc/liveaccess/client"
	objectclient "kc/object/client"
	snapshotclient "kc/snapshot/client"
)

// Client assembles typed service SDKs over the authenticated caller transport.
type Client struct {
	transport        *clientauth.Client
	baseURL          string
	snapshots        snapshotclient.Client
	objects          objectclient.Client
	discovery        discoveryclient.Client
	declarativeIndex declarativeindexclient.Client
	liveAccess       accessclient.Client
}

func New(config Config) (*Client, error) {
	transport, err := clientauth.New(config)
	if err != nil {
		return nil, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	c := &Client{transport: transport, baseURL: baseURL}
	snapshots, err := snapshotclient.New(snapshotclient.Config{BaseURL: baseURL, Transport: c})
	if err != nil {
		return nil, err
	}
	c.snapshots = snapshots
	objects, err := objectclient.New(objectclient.Config{BaseURL: baseURL, Transport: c})
	if err != nil {
		return nil, err
	}
	c.objects = objects
	discovery, err := discoveryclient.New(discoveryclient.Config{BaseURL: baseURL, Transport: c})
	if err != nil {
		return nil, err
	}
	c.discovery = discovery
	declarativeIndex, err := declarativeindexclient.New(declarativeindexclient.Config{BaseURL: baseURL, Transport: c})
	if err != nil {
		return nil, err
	}
	c.declarativeIndex = declarativeIndex
	access, err := accessclient.New(accessclient.Config{BaseURL: baseURL, Transport: c})
	if err != nil {
		return nil, err
	}
	c.liveAccess = access
	return c, nil
}

func (c *Client) LiveAccessService() accessclient.Client { return c.liveAccess }

func (c *Client) doJSON(ctx context.Context, method, path string, input any, options RequestOptions, output any) error {
	return httpjson.Do(ctx, c, c.baseURL, method, path, input, options.RequestID, output)
}

// Login establishes caller-local authentication through the shared transport.
func (c *Client) Login(ctx context.Context, request LoginRequest) (Identity, error) {
	return c.transport.Login(ctx, request)
}
func (c *Client) Logout(ctx context.Context) error { return c.transport.Logout(ctx) }
func (c *Client) Identity(ctx context.Context) (Identity, bool, error) {
	return c.transport.Identity(ctx)
}
func (c *Client) Do(ctx context.Context, audience string, request *http.Request) (*http.Response, error) {
	return c.transport.Do(ctx, audience, request)
}
func (c *Client) IdentityService() IdentityService { return c.transport.IdentityService() }
