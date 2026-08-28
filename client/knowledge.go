package client

import (
	"context"
	"encoding/json"
)

type KnowledgeService struct{ client *Client }

func (c *Client) KnowledgeService() KnowledgeService { return KnowledgeService{client: c} }

type KnowledgeReadRequest struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
	Object    string          `json:"object"`
	Aspect    string          `json:"aspect,omitempty"`
	Member    string          `json:"member,omitempty"`
	Include   []string        `json:"include,omitempty"`
	Exclude   []string        `json:"exclude,omitempty"`
}

type KnowledgeSearchRequest struct {
	Catalog      string          `json:"catalog,omitempty"`
	Workspace    string          `json:"workspace"`
	Pin          json.RawMessage `json:"pin,omitempty"`
	Query        string          `json:"query,omitempty"`
	Match        []string        `json:"match,omitempty"`
	MatchMode    string          `json:"matchMode,omitempty"`
	Equal        []string        `json:"equal,omitempty"`
	NotEqual     []string        `json:"notEqual,omitempty"`
	Sort         []string        `json:"sort,omitempty"`
	Limit        int             `json:"limit,omitempty"`
	Continuation string          `json:"continuation,omitempty"`
}

type KnowledgeRelationsRequest struct {
	Catalog      string          `json:"catalog,omitempty"`
	Workspace    string          `json:"workspace"`
	Pin          json.RawMessage `json:"pin,omitempty"`
	Endpoint     string          `json:"endpoint"`
	RelationType string          `json:"relationType,omitempty"`
	Role         string          `json:"role,omitempty"`
	Direction    string          `json:"direction,omitempty"`
	Limit        int             `json:"limit,omitempty"`
	Continuation string          `json:"continuation,omitempty"`
}

type KnowledgeObjectRequest struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
	Object    string          `json:"object"`
}

type KnowledgeSchemaRequest struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
	Object    string          `json:"object,omitempty"`
}

type KnowledgeBindingRequest struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
	Object    string          `json:"object"`
	Aspect    string          `json:"aspect"`
	Member    string          `json:"member,omitempty"`
}

func (s KnowledgeService) Read(ctx context.Context, request KnowledgeReadRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/objects:read", request, options, output)
}

func (s KnowledgeService) Search(ctx context.Context, request KnowledgeSearchRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/search", request, options, output)
}

func (s KnowledgeService) Relations(ctx context.Context, request KnowledgeRelationsRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/relations:query", request, options, output)
}

func (s KnowledgeService) Provenance(ctx context.Context, request KnowledgeObjectRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/provenance:get", request, options, output)
}

func (s KnowledgeService) Log(ctx context.Context, request KnowledgeObjectRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/log:get", request, options, output)
}

func (s KnowledgeService) Schema(ctx context.Context, request KnowledgeSchemaRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/schemas:get", request, options, output)
}

func (s KnowledgeService) ResolveBinding(ctx context.Context, request KnowledgeBindingRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/knowledge/v1/bindings:resolve", request, options, output)
}
