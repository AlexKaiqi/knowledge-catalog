package client

import "context"

type AdmissionResult struct {
	Principal      string   `json:"principal"`
	Catalog        string   `json:"catalog,omitempty"`
	Eligible       bool     `json:"eligible"`
	Status         string   `json:"status"`
	Actions        []string `json:"actions"`
	CurrentActions []string `json:"currentActions"`
}

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

func (s IdentityService) Admission(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/identity/v1/admission", nil, o, out)
}

func (s IdentityService) RequestAdmission(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/identity/v1/admission", struct{}{}, o, out)
}

func (s CatalogService) RepositoryShares(ctx context.Context, repository string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/catalog/v1/repositories/"+resourceSegment(repository)+"/shares", nil, o, out)
}

func (s CatalogService) ShareRepository(ctx context.Context, repository string, q RepositoryShareRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/catalog/v1/repositories/"+resourceSegment(repository)+"/shares", q, o, out)
}

func (s CatalogService) RevokeRepositoryShare(ctx context.Context, repository, share string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "DELETE", "/catalog/v1/repositories/"+resourceSegment(repository)+"/shares/"+resourceSegment(share), nil, o, out)
}
