package client

import "context"

type RepositoryShareRequest struct {
	Principal string   `json:"principal"`
	Actions   []string `json:"actions"`
}

type RepositoryShare struct {
	ID         string   `json:"id"`
	Repository string   `json:"repository"`
	Principal  string   `json:"principal"`
	Actions    []string `json:"actions"`
	SharedBy   string   `json:"sharedBy"`
}

type RepositoryShares struct {
	Repository     string            `json:"repository"`
	AllowedActions []string          `json:"allowedActions"`
	Shares         []RepositoryShare `json:"shares"`
}

func (s RepositoryManagementService) RepositoryShares(ctx context.Context, repository string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/snapshot/v1/connections/"+resourceSegment(repository)+"/shares", nil, o, out)
}

func (s RepositoryManagementService) ShareRepository(ctx context.Context, repository string, q RepositoryShareRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/connections/"+resourceSegment(repository)+"/shares", q, o, out)
}

func (s RepositoryManagementService) RevokeRepositoryShare(ctx context.Context, repository, share string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "DELETE", "/snapshot/v1/connections/"+resourceSegment(repository)+"/shares/"+resourceSegment(share), nil, o, out)
}
