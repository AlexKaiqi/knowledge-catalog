package client

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"kc/internal/httpjson"
)

type client struct {
	baseURL   string
	transport Transport
}

type service struct{ client *client }
type filesService struct{ client *client }
type accessService struct{ client *client }

func New(config Config) (Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("snapshot client base URL must be an http(s) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("snapshot client base URL must not contain credentials, query, or fragment")
	}
	if config.Transport == nil {
		return nil, fmt.Errorf("snapshot client requires an authenticated transport")
	}
	return &client{baseURL: baseURL, transport: config.Transport}, nil
}

func (c *client) SnapshotService() Service            { return service{client: c} }
func (c *client) WorkspaceFilesService() FilesService { return filesService{client: c} }
func (c *client) AdminService() AccessService         { return accessService{client: c} }

func (c *client) doJSON(ctx context.Context, method, path string, input any, options RequestOptions, output any) error {
	return httpjson.Do(ctx, c.transport, c.baseURL, method, path, input, options.RequestID, output)
}
