package client

import "context"

// ConnectionRequest is explicit authority identity plus an authorized private
// credential. Services must not reflect or journal this request body.
type ConnectionRequest struct {
	Repository string `json:"repository"`
	Driver     string `json:"driver"`
	URL        string `json:"url"`
	Credential string `json:"credential"`
}
type ConnectionRotationRequest struct {
	Credential string `json:"credential"`
}

func (s CatalogService) ConnectRepository(ctx context.Context, catalog string, q ConnectionRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/catalog/v1/catalogs/"+resourceSegment(catalog)+"/repositories:connect", q, o, out)
}
func (s CatalogService) RepositoryConnection(ctx context.Context, repository string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/catalog/v1/repositories/"+resourceSegment(repository)+"/connection", nil, o, out)
}
func (s CatalogService) CheckRepositoryConnection(ctx context.Context, repository string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/catalog/v1/repositories/"+resourceSegment(repository)+"/connection:check", struct{}{}, o, out)
}
func (s CatalogService) RotateRepositoryConnection(ctx context.Context, repository string, q ConnectionRotationRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/catalog/v1/repositories/"+resourceSegment(repository)+"/connection:rotate", q, o, out)
}
