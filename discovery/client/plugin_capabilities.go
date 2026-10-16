package client

import (
	"context"
	"net/url"

	retrieval "kc/discovery"
)

// Trial request shapes travel with the transport the CLI already depends on.
type DeclarationTrialRequest = retrieval.DeclarationTrialRequest
type DeclarationDocument = retrieval.DeclarationDocument

func (c *client) PluginCapabilities(ctx context.Context, name string, options RequestOptions, output any) error {
	return c.request(ctx, "GET", "/object/v1/plugins/"+url.PathEscape(name)+"/capabilities", nil, options, output)
}

func (c *client) TrialDeclarations(ctx context.Context, name string, request retrieval.DeclarationTrialRequest, options RequestOptions, output any) error {
	return c.request(ctx, "POST", "/object/v1/plugins/"+url.PathEscape(name)+"/declarations:trial", request, options, output)
}
