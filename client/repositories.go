package client

import (
	"context"
	"strings"

	"kc/kernel"
)

// NamedRepositoryCreateRequest contains user choices only. Server chooses the
// logical coordinates, owns the allocation command, and provisions credentials.
type NamedRepositoryCreateRequest struct {
	Name    string `json:"name"`
	Store   string `json:"store,omitempty"`
	Catalog string `json:"catalog,omitempty"`
}

func (s CatalogService) CreateNamedRepository(ctx context.Context, q NamedRepositoryCreateRequest, o RequestOptions, out any) error {
	if strings.TrimSpace(q.Name) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "repository creation requires a name")
	}
	return s.client.doJSON(ctx, "POST", "/catalog/v1/repositories", q, o, out)
}

func (s CatalogService) MyRepositories(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/catalog/v1/repositories", nil, o, out)
}

func (s CatalogService) ManagedRepository(ctx context.Context, repository string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/catalog/v1/repositories/"+resourceSegment(repository), nil, o, out)
}
