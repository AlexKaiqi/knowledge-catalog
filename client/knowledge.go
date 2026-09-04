package client

import (
	"context"
	"encoding/json"

	"kc/knowledge"
	"kc/retrieval"
)

type KnowledgeService struct{ client *Client }

func (c *Client) KnowledgeService() KnowledgeService { return KnowledgeService{client: c} }

type KnowledgeReadRequest struct {
	Catalog    string          `json:"catalog,omitempty"`
	Workspace  string          `json:"workspace,omitempty"`
	Pin        json.RawMessage `json:"pin,omitempty"`
	Repository string          `json:"repository,omitempty"`
	Commit     string          `json:"commit,omitempty"`
	Ref        string          `json:"ref,omitempty"`
	Object     string          `json:"object"`
	Aspect     string          `json:"aspect,omitempty"`
	Member     string          `json:"member,omitempty"`
	Include    []string        `json:"include,omitempty"`
	Exclude    []string        `json:"exclude,omitempty"`
}

type KnowledgeSearchRequest struct {
	Catalog      string                  `json:"catalog,omitempty"`
	Workspace    string                  `json:"workspace"`
	Pin          json.RawMessage         `json:"pin,omitempty"`
	Query        string                  `json:"query,omitempty"`
	Match        []string                `json:"match,omitempty"`
	MatchMode    string                  `json:"matchMode,omitempty"`
	Equal        []string                `json:"equal,omitempty"`
	NotEqual     []string                `json:"notEqual,omitempty"`
	In           []string                `json:"in,omitempty"`
	Exists       []string                `json:"exists,omitempty"`
	Missing      []string                `json:"missing,omitempty"`
	Prefix       []string                `json:"prefix,omitempty"`
	Contains     []string                `json:"contains,omitempty"`
	GreaterThan  []string                `json:"greaterThan,omitempty"`
	GreaterEqual []string                `json:"greaterEqual,omitempty"`
	LessThan     []string                `json:"lessThan,omitempty"`
	LessEqual    []string                `json:"lessEqual,omitempty"`
	Sort         []string                `json:"sort,omitempty"`
	Limit        int                     `json:"limit,omitempty"`
	Continuation string                  `json:"continuation,omitempty"`
	Expression   *retrieval.SearchExpr   `json:"expression,omitempty"`
	Order        *retrieval.SearchClause `json:"order,omitempty"`
}

type KnowledgeRelationsRequest struct {
	Catalog      string          `json:"catalog,omitempty"`
	Workspace    string          `json:"workspace,omitempty"`
	Pin          json.RawMessage `json:"pin,omitempty"`
	Repository   string          `json:"repository,omitempty"`
	Commit       string          `json:"commit,omitempty"`
	Ref          string          `json:"ref,omitempty"`
	Endpoint     string          `json:"endpoint"`
	RelationType string          `json:"relationType,omitempty"`
	Role         string          `json:"role,omitempty"`
	Direction    string          `json:"direction,omitempty"`
	Limit        int             `json:"limit,omitempty"`
	Continuation string          `json:"continuation,omitempty"`
}

type KnowledgeRerankRequest struct {
	Catalog    string                         `json:"catalog,omitempty"`
	Workspace  string                         `json:"workspace"`
	Pin        json.RawMessage                `json:"pin,omitempty"`
	Candidates []knowledge.KnowledgeRef       `json:"candidates"`
	Spec       retrieval.SemanticOperatorSpec `json:"spec"`
}

type KnowledgeSearchRerankRequest struct {
	KnowledgeSearchRequest
	Spec retrieval.SemanticOperatorSpec `json:"spec"`
}

type KnowledgeObjectRequest struct {
	Catalog      string          `json:"catalog,omitempty"`
	Workspace    string          `json:"workspace,omitempty"`
	Pin          json.RawMessage `json:"pin,omitempty"`
	Repository   string          `json:"repository,omitempty"`
	Commit       string          `json:"commit,omitempty"`
	Ref          string          `json:"ref,omitempty"`
	Object       string          `json:"object"`
	Limit        int             `json:"limit,omitempty"`
	Continuation string          `json:"continuation,omitempty"`
}

type KnowledgeResolveRequest struct {
	Catalog    string          `json:"catalog,omitempty"`
	Workspace  string          `json:"workspace,omitempty"`
	Pin        json.RawMessage `json:"pin,omitempty"`
	Repository string          `json:"repository,omitempty"`
	Commit     string          `json:"commit,omitempty"`
	Ref        string          `json:"ref,omitempty"`
	Object     string          `json:"object"`
	Aspect     string          `json:"aspect,omitempty"`
	Member     string          `json:"member,omitempty"`
}

type KnowledgeSchemaRequest struct {
	Catalog    string          `json:"catalog,omitempty"`
	Workspace  string          `json:"workspace,omitempty"`
	Pin        json.RawMessage `json:"pin,omitempty"`
	Repository string          `json:"repository,omitempty"`
	Commit     string          `json:"commit,omitempty"`
	Ref        string          `json:"ref,omitempty"`
	Object     string          `json:"object,omitempty"`
}

type KnowledgeBindingRequest struct {
	Catalog    string          `json:"catalog,omitempty"`
	Workspace  string          `json:"workspace,omitempty"`
	Pin        json.RawMessage `json:"pin,omitempty"`
	Repository string          `json:"repository,omitempty"`
	Commit     string          `json:"commit,omitempty"`
	Ref        string          `json:"ref,omitempty"`
	Object     string          `json:"object"`
	Aspect     string          `json:"aspect"`
	Member     string          `json:"member,omitempty"`
}

// KnowledgeResourceAccessRequest invokes a stable ResourceDescriptor operation
// or hydrates one pinned Binding through the Knowledge Server's configured
// resource-access/v1 runtime. Input remains raw JSON so number precision and
// provider-specific object shapes survive the client hop unchanged.
type KnowledgeResourceAccessRequest struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace,omitempty"`
	Pin       json.RawMessage `json:"pin,omitempty"`
	Object    string          `json:"object"`
	Aspect    string          `json:"aspect,omitempty"`
	Member    string          `json:"member,omitempty"`
	Operation string          `json:"operation,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
}

// KnowledgeSchemaPageRequest discovers Domain Schemas at one fixed
// Repository basis before a consumer has selected a Workspace.
type KnowledgeSchemaPageRequest struct {
	Repository   string `json:"repository"`
	Commit       string `json:"commit,omitempty"`
	Ref          string `json:"ref,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

func (s KnowledgeService) Read(ctx context.Context, request KnowledgeReadRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/objects:read", request, options, output)
}

func (s KnowledgeService) Search(ctx context.Context, request KnowledgeSearchRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/search", request, options, output)
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
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/relations:query", request, options, output)
}

func (s KnowledgeService) Provenance(ctx context.Context, request KnowledgeObjectRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/provenance:describe", request, options, output)
}

func (s KnowledgeService) Log(ctx context.Context, request KnowledgeObjectRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/log:query", request, options, output)
}

func (s KnowledgeService) Resolve(ctx context.Context, request KnowledgeResolveRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/objects:resolve", request, options, output)
}

func (s KnowledgeService) Schema(ctx context.Context, request KnowledgeSchemaRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/schemas:describe", request, options, output)
}

func (s KnowledgeService) BrowseSchemas(ctx context.Context, request KnowledgeSchemaPageRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/schemas:list", request, options, output)
}

func (s KnowledgeService) ResolveBinding(ctx context.Context, request KnowledgeBindingRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/bindings:resolve", request, options, output)
}

func (s KnowledgeService) AccessResource(ctx context.Context, request KnowledgeResourceAccessRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/resources:access", request, options, output)
}
