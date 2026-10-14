package client

import (
	"context"
	"encoding/json"
	"kc/kernel"
	"kc/snapshot/dataset"
	"net/url"
	"strconv"
)

func resourceSegment(value string) string { return url.PathEscape(value) }

type DatasetRequest struct {
	Dataset  string                  `json:"dataset"`
	Revision int                     `json:"revision"`
	Sources  []dataset.DatasetSource `json:"sources"`
	Metadata map[string]string       `json:"metadata,omitempty"`
}
type DatasetResolveRequest struct {
	Pin                json.RawMessage `json:"pin,omitempty"`
	Revision           *int            `json:"revision,omitempty"`
	InventoryDiscovery bool            `json:"inventoryDiscovery,omitempty"`
}

// DatasetShowOptions selects one accepted release and one page of its
// published history. Nil Revision selects the current serving release.
type DatasetShowOptions struct {
	Revision     *int
	Limit        int
	Continuation string
}
type RepositoryAttachRequest struct {
	Repository string `json:"repository"`
}

func (s service) Show(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/snapshot/v1/inventory", nil, o, out)
}
func (s service) Audit(ctx context.Context, limit int, continuation string, o RequestOptions, out any) error {
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if continuation != "" {
		query.Set("continuation", continuation)
	}
	path := "/snapshot/v1/audit"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	return s.client.doJSON(ctx, "GET", path, nil, o, out)
}
func (s service) Repositories(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/snapshot/v1/repositories", nil, o, out)
}
func (s service) AttachRepository(ctx context.Context, q RepositoryAttachRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/repositories", q, o, out)
}
func (s service) DetachRepository(ctx context.Context, repository string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "DELETE", "/snapshot/v1/repositories/"+resourceSegment(repository), nil, o, out)
}
func (s service) Datasets(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/snapshot/v1/datasets", nil, o, out)
}
func (s service) DefineDataset(ctx context.Context, q DatasetRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/datasets", q, o, out)
}
func (s service) ResolveDefinition(ctx context.Context, q DatasetRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/datasets:resolve", q, o, out)
}
func (s service) Dataset(ctx context.Context, setID string, o RequestOptions, out any) error {
	return s.ShowDataset(ctx, setID, DatasetShowOptions{}, o, out)
}

func (s service) ShowDataset(ctx context.Context, setID string, query DatasetShowOptions, o RequestOptions, out any) error {
	values := url.Values{}
	if query.Revision != nil {
		if *query.Revision <= 0 {
			return kernel.Fail(kernel.ErrUsageInvalid, "revision must be a positive integer")
		}
		values.Set("revision", strconv.Itoa(*query.Revision))
	}
	if query.Limit != 0 {
		values.Set("limit", strconv.Itoa(query.Limit))
	}
	if query.Continuation != "" {
		values.Set("continuation", query.Continuation)
	}
	path := "/snapshot/v1/datasets/" + resourceSegment(setID)
	if len(values) != 0 {
		path += "?" + values.Encode()
	}
	return s.client.doJSON(ctx, "GET", path, nil, o, out)
}
func (s service) RetireDataset(ctx context.Context, workspace string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/datasets/"+resourceSegment(workspace)+"/retire", struct{}{}, o, out)
}
func (s service) ResolveDataset(ctx context.Context, setID string, q DatasetResolveRequest, o RequestOptions, out any) error {
	if err := validateDatasetResolveRequest(q); err != nil {
		return err
	}
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/datasets/"+resourceSegment(setID)+"/resolve", q, o, out)
}
func (s service) CheckDataset(ctx context.Context, setID string, q DatasetResolveRequest, o RequestOptions, out any) error {
	if q.Revision != nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "revision is only valid for Dataset resolve")
	}
	if err := validateDatasetResolveRequest(q); err != nil {
		return err
	}
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/datasets/"+resourceSegment(setID)+"/check", q, o, out)
}

func validateDatasetResolveRequest(request DatasetResolveRequest) error {
	if request.Revision != nil {
		if *request.Revision <= 0 {
			return kernel.Fail(kernel.ErrUsageInvalid, "revision must be a positive integer")
		}
		if len(request.Pin) != 0 {
			return kernel.Fail(kernel.ErrUsageInvalid, "revision and pin are mutually exclusive")
		}
	}
	return nil
}
