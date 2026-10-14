package client

import (
	"context"
	"kc/kernel"
	"net/url"
	"strings"
)

type GrantRequest struct {
	Principal  string   `json:"principal"`
	Actions    []string `json:"actions"`
	Repository string   `json:"repository,omitempty"`
	Ref        string   `json:"ref,omitempty"`
	Object     string   `json:"object,omitempty"`
	Aspect     string   `json:"aspect,omitempty"`
	Dataset    string   `json:"dataset,omitempty"`
}

func (s accessService) AddGrant(ctx context.Context, q GrantRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/admin/v1/grants", q, o, out)
}
func (s accessService) Grants(ctx context.Context, o RequestOptions, out any) error {
	return s.GrantsMatching(ctx, GrantRequest{}, o, out)
}

// GrantsMatching reuses grant coordinates for inventory filters. Actions may
// contain one requested semantic action; this remains a list, not evaluation.
func (s accessService) GrantsMatching(ctx context.Context, q GrantRequest, o RequestOptions, out any) error {
	if len(q.Actions) > 1 {
		return kernel.Fail(kernel.ErrUsageInvalid, "grant inventory accepts at most one action filter")
	}
	query := url.Values{}
	for key, value := range map[string]string{"principal": q.Principal, "repository": q.Repository, "dataset": q.Dataset, "ref": q.Ref, "object": q.Object, "aspect": q.Aspect} {
		if value != "" {
			query.Set(key, value)
		}
	}
	if len(q.Actions) == 1 {
		query.Set("action", q.Actions[0])
	}
	path := "/admin/v1/grants"
	if len(query) != 0 {
		path += "?" + query.Encode()
	}
	return s.client.doJSON(ctx, "GET", path, nil, o, out)
}
func (s accessService) RemoveGrant(ctx context.Context, id string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "DELETE", "/admin/v1/grants/"+resourceSegment(id), nil, o, out)
}

// GrantExpected is the previously observed public rule in the Admin PUT vocabulary.
// Its identity and provenance remain the server's authority.
type GrantExpected struct {
	GrantRequest
	ID       string `json:"id"`
	SharedBy string `json:"sharedBy,omitempty"`
	ShareID  string `json:"shareId,omitempty"`
}

// GrantUpdateRequest supplies a full replacement of editable rule fields and
// the caller's earlier observation. It does not fetch or refresh that observation.
type GrantUpdateRequest struct {
	GrantRequest
	Expected GrantExpected `json:"expected"`
}

func (s accessService) UpdateGrant(ctx context.Context, id string, request GrantUpdateRequest, options RequestOptions, output any) error {
	if strings.TrimSpace(id) == "" || request.Expected.ID != id {
		return kernel.Fail(kernel.ErrUsageInvalid, "grant update requires matching rule and expected IDs")
	}
	return s.client.doJSON(ctx, "PUT", "/admin/v1/grants/"+resourceSegment(id), request, options, output)
}

type AdmissionResult struct {
	Principal string           `json:"principal"`
	Grants    []AdmissionGrant `json:"grants"`
	Request   AdmissionRequest `json:"request"`
}

type AdmissionGrant struct {
	ID         string   `json:"id"`
	Actions    []string `json:"actions"`
	Repository string   `json:"repository,omitempty"`
	Dataset    string   `json:"dataset,omitempty"`
	Ref        string   `json:"ref,omitempty"`
	Object     string   `json:"object,omitempty"`
	Aspect     string   `json:"aspect,omitempty"`
	SharedBy   string   `json:"sharedBy,omitempty"`
}

type AdmissionRequest struct {
	URL            string   `json:"url,omitempty"`
	Administrators []string `json:"administrators"`
}

func (s accessService) Admission(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/identity/v1/admission", nil, o, out)
}
