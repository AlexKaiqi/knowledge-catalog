package client

import (
	"context"
	"net/http"
)

// RepositoryMetadata reads native lakeFS metadata. repository is the actual
// native repository name exposed by the configured instance, not a logical ID.
func (s service) RepositoryMetadata(ctx context.Context, repository string, options RequestOptions) (map[string]string, error) {
	var metadata map[string]string
	err := s.client.doJSON(ctx, http.MethodGet, nativeRepositoryMetadataPath(repository), nil, options, &metadata)
	return metadata, err
}

// SetRepositoryMetadata adds or updates only the provided keys. Metadata stays
// authoritative in lakeFS and does not update registration or file commits.
func (s service) SetRepositoryMetadata(ctx context.Context, repository string, metadata map[string]string, options RequestOptions) error {
	if metadata == nil {
		metadata = map[string]string{}
	}
	request := struct {
		Metadata map[string]string `json:"metadata"`
	}{Metadata: metadata}
	return s.client.doJSON(ctx, http.MethodPost, nativeRepositoryMetadataPath(repository), request, options, nil)
}

// DeleteRepositoryMetadata removes only the specified keys using the native
// DELETE body; it does not delete or detach the repository itself.
func (s service) DeleteRepositoryMetadata(ctx context.Context, repository string, keys []string, options RequestOptions) error {
	if keys == nil {
		keys = []string{}
	}
	request := struct {
		Keys []string `json:"keys"`
	}{Keys: keys}
	return s.client.doJSON(ctx, http.MethodDelete, nativeRepositoryMetadataPath(repository), request, options, nil)
}

func nativeRepositoryMetadataPath(repository string) string {
	return "/api/v1/repositories/" + resourceSegment(repository) + "/metadata"
}
