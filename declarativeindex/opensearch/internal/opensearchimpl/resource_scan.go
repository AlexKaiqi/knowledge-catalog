package opensearchimpl

import (
	"context"
	"net/http"

	"kc/kernel"
)

type resourceHit struct {
	ID     string     `json:"_id"`
	Source controlDoc `json:"_source"`
	Sort   []any      `json:"sort"`
}

func (e *openSearchEngine) scanResourcePage(ctx context.Context, query any, sort, after []any) ([]resourceHit, error) {
	request := map[string]any{"size": 128, "query": query, "sort": sort, "track_total_hits": false}
	if len(after) > 0 {
		request["search_after"] = after
	}
	status, body, err := e.doContext(ctx, http.MethodPost, "/"+e.controlIndexPath()+"/_search", request)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status >= 300 {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "resource authority enumeration returned HTTP %d: %s", status, body)
	}
	var response struct {
		TimedOut bool           `json:"timed_out"`
		Shards   *shardResponse `json:"_shards"`
		Hits     struct {
			Hits []resourceHit `json:"hits"`
		} `json:"hits"`
	}
	if err := decodeJSON(body, &response); err != nil {
		return nil, err
	}
	if response.TimedOut {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "resource authority enumeration timed out")
	}
	if err := response.Shards.check("resource recovery"); err != nil {
		return nil, err
	}
	return response.Hits.Hits, nil
}

func resourceNextCursor(hits []resourceHit, previous []any, width int) ([]any, error) {
	next := hits[len(hits)-1].Sort
	if len(previous) > 0 && kernel.CanonicalDigest(next) == kernel.CanonicalDigest(previous) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "resource recovery did not advance")
	}
	if err := validateResourceCursor(next, width); err != nil {
		return nil, err
	}
	return next, nil
}

// resourceScope reconstructs only immutable provider coordinates, never an
// engine mutex, query handle or build state. Ownership is derived from the
// actual namespace and full engine identity before using a physical name.
func (e *openSearchEngine) resourceScope(repository kernel.RepositoryID) *openSearchEngine {
	prefix, controlID := projectionNames(repository)
	if e.namespace != "" {
		prefix = "kc-proj-" + string(namespaceDigest(e.namespace))[:24] + "-" + controlID[:24]
	}
	return &openSearchEngine{base: e.base, http: e.http, user: e.user, pass: e.pass, apiKey: e.apiKey,
		controlIndex: e.controlIndex, namespace: e.namespace, repository: repository, prefix: prefix, controlID: controlID}
}

func (e *openSearchEngine) resourceSession(hit resourceHit, instance bool, ctx context.Context) (*rebuildSession, error) {
	owner := e
	if instance {
		owner = e.resourceScope(kernel.RepositoryID(hit.Source.Repository))
	}
	record := hit.Source
	if record.Repository == "" || record.Repository != string(owner.repository) || !validGeneration(record.Generation) || hit.ID != owner.controlID+"-resource-"+record.Generation || record.ActiveIndex != owner.prefix+"-g-"+record.Generation {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "resource recovery found inconsistent ownership: %s", hit.ID)
	}
	return &rebuildSession{engine: owner, physicalIndex: record.ActiveIndex, generation: record.Generation, resourceContext: ctx}, nil
}

func validateResourceCursor(cursor []any, width int) error {
	if len(cursor) != width {
		return kernel.Fail(kernel.ErrPreconditionFailed, "resource recovery has invalid cursor width")
	}
	for _, value := range cursor {
		if key, ok := value.(string); !ok || key == "" {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource recovery returned invalid sort keys")
		}
	}
	return nil
}
