package client

import (
	"context"
	"strings"

	"kc/kernel"
)

// NamedRepositoryCreateRequest contains user choices only. Server chooses the
// logical coordinates, owns the allocation command, and provisions credentials.
type NamedRepositoryCreateRequest struct {
	Name  string `json:"name"`
	Store string `json:"store,omitempty"`
}

func (s SnapshotService) CreateNamedRepository(ctx context.Context, q NamedRepositoryCreateRequest, o RequestOptions, out any) error {
	if strings.TrimSpace(q.Name) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "repository creation requires a name")
	}
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/connections", q, o, out)
}

func (s SnapshotService) MyRepositories(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/snapshot/v1/connections", nil, o, out)
}

func (s SnapshotService) ManagedRepository(ctx context.Context, repository string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/snapshot/v1/connections/"+resourceSegment(repository), nil, o, out)
}
