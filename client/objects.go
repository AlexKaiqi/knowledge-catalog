package client

import (
	"context"
	accessclient "kc/liveaccess/client"

	discoveryclient "kc/discovery/client"
	objectclient "kc/object/client"
)

type Objects struct{ client *Client }

func (c *Client) Objects() Objects { return Objects{client: c} }

// ObjectDatasetIdentity is the shared dataset-channel identity envelope:
// a named workspace, or an inline published pin/definition.
type ObjectDatasetIdentity = objectclient.ObjectDatasetIdentity

// ObjectRepoPin is the shared repository-channel basis envelope: one
// fixed commit (or ref resolved at the server) of one repository.
type ObjectRepoPin = objectclient.ObjectRepoPin

// ObjectScope is the full scope envelope of object API requests: the
// dataset identity or the repository basis used by scope-oriented operations.
// Basic object reads instead carry their source identity in the complete
// object Reference and keep only Dataset and version qualifiers alongside it.
type ObjectScope = objectclient.ObjectScope

type ObjectReadRequest = objectclient.ReadRequest

type ObjectSearchRequest = discoveryclient.SearchRequest

type ObjectRelationsRequest = discoveryclient.RelationsRequest

// ObjectTraverseRequest runs one bounded neighborhood closure on a fixed
// basis. MaxHops is required; the server fixes the scope before execution and
// never widens it to out-of-scope frontiers.
type ObjectTraverseRequest = discoveryclient.TraverseRequest

type ObjectRerankRequest = discoveryclient.RerankRequest

type ObjectSearchRerankRequest = discoveryclient.SearchRerankRequest

type ObjectRequest = objectclient.ObjectRequest

type ObjectResolveRequest = objectclient.ResolveRequest

type ObjectSchemaRequest = objectclient.SchemaRequest

type ObjectBindingRequest = accessclient.BindingRequest

// ObjectResourceAccessRequest selects an object Interface implementation,
// invokes a compatible ResourceDescriptor operation, or hydrates a pinned
// Binding. Input remains raw JSON across the client hop; handlers are installed
// at the Server, not shipped by the consumer.
type ObjectResourceAccessRequest = accessclient.AccessRequest

// ObjectSchemaPageRequest discovers Domain Schemas at one fixed
// Repository basis before a consumer has selected a Workspace.
type ObjectSchemaPageRequest = objectclient.SchemaPageRequest

func (s Objects) Read(ctx context.Context, request ObjectReadRequest, options RequestOptions, output any) error {
	return s.client.objects.Read(ctx, request, objectclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s Objects) Search(ctx context.Context, request ObjectSearchRequest, options RequestOptions, output any) error {
	return s.client.discovery.Search(ctx, request, discoveryclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s Objects) Rerank(ctx context.Context, request ObjectRerankRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/object/v1/rerank", request, options, output)
}

// SearchRerank executes one server-side fixed-view candidate window followed
// by one listwise semantic rerank. It is a thin physical composition, not a
// query planner, and the response retains evidence for both stages.
func (s Objects) SearchRerank(ctx context.Context, request ObjectSearchRerankRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/object/v1/search:rerank", request, options, output)
}

func (s Objects) Relations(ctx context.Context, request ObjectRelationsRequest, options RequestOptions, output any) error {
	return s.client.discovery.Relations(ctx, request, discoveryclient.RequestOptions{RequestID: options.RequestID}, output)
}

// Traverse runs one bounded neighborhood closure. The server owns the hop
// ceiling, the edge budget, and the fixed scope; out-of-scope frontiers come
// back as explicit boundaries instead of nodes.
func (s Objects) Traverse(ctx context.Context, request ObjectTraverseRequest, options RequestOptions, output any) error {
	return s.client.discovery.Traverse(ctx, request, discoveryclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s Objects) Provenance(ctx context.Context, request ObjectRequest, options RequestOptions, output any) error {
	return s.client.objects.Provenance(ctx, request, objectclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s Objects) Log(ctx context.Context, request ObjectRequest, options RequestOptions, output any) error {
	return s.client.objects.Log(ctx, request, objectclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s Objects) Resolve(ctx context.Context, request ObjectResolveRequest, options RequestOptions, output any) error {
	return s.client.objects.Resolve(ctx, request, objectclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s Objects) Schema(ctx context.Context, request ObjectSchemaRequest, options RequestOptions, output any) error {
	return s.client.objects.Schema(ctx, request, objectclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s Objects) BrowseSchemas(ctx context.Context, request ObjectSchemaPageRequest, options RequestOptions, output any) error {
	return s.client.objects.BrowseSchemas(ctx, request, objectclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s Objects) ResolveBinding(ctx context.Context, request ObjectBindingRequest, options RequestOptions, output any) error {
	return s.client.liveAccess.ResolveBinding(ctx, request, accessclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s Objects) AccessResource(ctx context.Context, request ObjectResourceAccessRequest, options RequestOptions, output any) error {
	return s.client.liveAccess.AccessResource(ctx, request, accessclient.RequestOptions{RequestID: options.RequestID}, output)
}
