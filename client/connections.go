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

func (s RepositoryManagementService) ConnectRepository(ctx context.Context, q ConnectionRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/connections:connect", q, o, out)
}
func (s RepositoryManagementService) RepositoryConnection(ctx context.Context, repository string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/snapshot/v1/connections/"+resourceSegment(repository)+"/connection", nil, o, out)
}
func (s RepositoryManagementService) CheckRepositoryConnection(ctx context.Context, repository string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/connections/"+resourceSegment(repository)+"/connection:check", struct{}{}, o, out)
}
func (s RepositoryManagementService) RotateRepositoryConnection(ctx context.Context, repository string, q ConnectionRotationRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/connections/"+resourceSegment(repository)+"/connection:rotate", q, o, out)
}
