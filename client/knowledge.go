package client

import (
	"context"
	accessclient "kc/liveaccess/client"

	discoveryclient "kc/discovery/client"
	objectclient "kc/object/client"
)

type KnowledgeService struct{ client *Client }

func (c *Client) KnowledgeService() KnowledgeService { return KnowledgeService{client: c} }

// KnowledgeDatasetIdentity is the shared dataset-channel identity envelope:
// a named workspace, or an inline published pin/definition.
type KnowledgeDatasetIdentity = objectclient.KnowledgeDatasetIdentity

// KnowledgeRepoPin is the shared repository-channel basis envelope: one
// fixed commit (or ref resolved at the server) of one repository.
type KnowledgeRepoPin = objectclient.KnowledgeRepoPin

// KnowledgeScope is the full scope envelope of knowledge API requests: the
// dataset identity or the repository basis used by scope-oriented operations.
// Basic object reads instead carry their source identity in the complete
// object Reference and keep only Dataset and version qualifiers alongside it.
type KnowledgeScope = objectclient.KnowledgeScope

type KnowledgeReadRequest = objectclient.ReadRequest

type KnowledgeSearchRequest = discoveryclient.SearchRequest

type KnowledgeRelationsRequest = discoveryclient.RelationsRequest

// KnowledgeTraverseRequest runs one bounded neighborhood closure on a fixed
// basis. MaxHops is required; the server fixes the scope before execution and
// never widens it to out-of-scope frontiers.
type KnowledgeTraverseRequest = discoveryclient.TraverseRequest

type KnowledgeRerankRequest = discoveryclient.RerankRequest

type KnowledgeSearchRerankRequest = discoveryclient.SearchRerankRequest

type KnowledgeObjectRequest = objectclient.ObjectRequest

type KnowledgeResolveRequest = objectclient.ResolveRequest

type KnowledgeSchemaRequest = objectclient.SchemaRequest

type KnowledgeBindingRequest = accessclient.BindingRequest

// KnowledgeResourceAccessRequest selects an object Interface implementation,
// invokes a compatible ResourceDescriptor operation, or hydrates a pinned
// Binding. Input remains raw JSON across the client hop; handlers are installed
// at the Server, not shipped by the consumer.
type KnowledgeResourceAccessRequest = accessclient.AccessRequest

// KnowledgeSchemaPageRequest discovers Domain Schemas at one fixed
// Repository basis before a consumer has selected a Workspace.
type KnowledgeSchemaPageRequest = objectclient.SchemaPageRequest

func (s KnowledgeService) Read(ctx context.Context, request KnowledgeReadRequest, options RequestOptions, output any) error {
	return s.client.objects.Read(ctx, request, objectclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s KnowledgeService) Search(ctx context.Context, request KnowledgeSearchRequest, options RequestOptions, output any) error {
	return s.client.discovery.Search(ctx, request, discoveryclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s KnowledgeService) Rerank(ctx context.Context, request KnowledgeRerankRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/rerank", request, options, output)
}

// SearchRerank executes one server-side fixed-view candidate window followed
// by one listwise semantic rerank. It is a thin physical composition, not a
// query planner, and the response retains evidence for both stages.
func (s KnowledgeService) SearchRerank(ctx context.Context, request KnowledgeSearchRerankRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/search:rerank", request, options, output)
}

func (s KnowledgeService) Relations(ctx context.Context, request KnowledgeRelationsRequest, options RequestOptions, output any) error {
	return s.client.discovery.Relations(ctx, request, discoveryclient.RequestOptions{RequestID: options.RequestID}, output)
}

// Traverse runs one bounded neighborhood closure. The server owns the hop
// ceiling, the edge budget, and the fixed scope; out-of-scope frontiers come
// back as explicit boundaries instead of nodes.
func (s KnowledgeService) Traverse(ctx context.Context, request KnowledgeTraverseRequest, options RequestOptions, output any) error {
	return s.client.discovery.Traverse(ctx, request, discoveryclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s KnowledgeService) Provenance(ctx context.Context, request KnowledgeObjectRequest, options RequestOptions, output any) error {
	return s.client.objects.Provenance(ctx, request, objectclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s KnowledgeService) Log(ctx context.Context, request KnowledgeObjectRequest, options RequestOptions, output any) error {
	return s.client.objects.Log(ctx, request, objectclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s KnowledgeService) Resolve(ctx context.Context, request KnowledgeResolveRequest, options RequestOptions, output any) error {
	return s.client.objects.Resolve(ctx, request, objectclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s KnowledgeService) Schema(ctx context.Context, request KnowledgeSchemaRequest, options RequestOptions, output any) error {
	return s.client.objects.Schema(ctx, request, objectclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s KnowledgeService) BrowseSchemas(ctx context.Context, request KnowledgeSchemaPageRequest, options RequestOptions, output any) error {
	return s.client.objects.BrowseSchemas(ctx, request, objectclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s KnowledgeService) ResolveBinding(ctx context.Context, request KnowledgeBindingRequest, options RequestOptions, output any) error {
	return s.client.liveAccess.ResolveBinding(ctx, request, accessclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s KnowledgeService) AccessResource(ctx context.Context, request KnowledgeResourceAccessRequest, options RequestOptions, output any) error {
	return s.client.liveAccess.AccessResource(ctx, request, accessclient.RequestOptions{RequestID: options.RequestID}, output)
}
