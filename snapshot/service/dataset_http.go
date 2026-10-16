package service

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

func (h *handler) registerDatasetRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /snapshot/v1/repositories", h.repositoryList)
	mux.HandleFunc("GET /snapshot/v1/datasets", h.datasetList)
	mux.HandleFunc("POST /snapshot/v1/datasets", h.datasetDefine)
	mux.HandleFunc("POST /snapshot/v1/datasets:resolve", h.datasetResolveDefinition)
	mux.HandleFunc("GET /snapshot/v1/datasets/{dataset}", h.datasetShow)
	mux.HandleFunc("POST /snapshot/v1/datasets/{dataset}/retire", h.datasetRetire)
	mux.HandleFunc("POST /snapshot/v1/datasets/{dataset}/resolve", h.datasetResolve)
	mux.HandleFunc("POST /snapshot/v1/datasets/{dataset}/check", h.datasetCheck)
}

type datasetRequest struct {
	Dataset  string                  `json:"dataset"`
	Revision int                     `json:"revision"`
	Sources  []dataset.DatasetSource `json:"sources"`
	Metadata datasetMetadata         `json:"metadata,omitempty"`
}

type datasetMetadata map[string]string

func (metadata *datasetMetadata) UnmarshalJSON(raw []byte) error {
	var values map[string]string
	if err := dataset.DecodeJSON(raw, &values); err != nil {
		return err
	}
	*metadata = values
	return nil
}

type datasetResolveRequest struct {
	Pin                json.RawMessage `json:"pin,omitempty"`
	Revision           json.RawMessage `json:"revision,omitempty"`
	InventoryDiscovery bool            `json:"inventoryDiscovery,omitempty"`
}

func openAuthorizedDatasets(ctx context.Context, caller identity.HTTPIdentity, resources Resources, datasetID, action string) (dataset.Service, error) {
	if err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: action, Dataset: datasetID}}); err != nil {
		return nil, err
	}
	return openDatasets(resources)
}

func openDatasets(resources Resources) (dataset.Service, error) {
	if resources.Datasets == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Dataset authority is required")
	}
	return resources.Datasets, nil
}

func (h *handler) datasetList(w http.ResponseWriter, r *http.Request) {
	h.withResources(w, r, "dataset-list", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		cat, err := openAuthorizedDatasets(ctx, caller, resources, "", "inventory.read")
		if err != nil {
			return nil, err
		}
		state := dataset.NormalizeState(cat.DumpState())
		datasets, err := consumableDatasets(ctx, caller, resources, state.Datasets)
		if err != nil {
			return nil, err
		}
		return map[string]any{"datasets": datasets}, nil
	})
}

func managementOptions(r *http.Request) (int, dataset.ManagementOptions, error) {
	revision := 0
	var err error
	if raw := r.URL.Query().Get("revision"); raw != "" {
		revision, err = strconv.Atoi(raw)
		if err != nil || revision <= 0 {
			return 0, dataset.ManagementOptions{}, kernel.Fail(kernel.ErrUsageInvalid, "revision must be a positive integer")
		}
	}
	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 0 {
			return 0, dataset.ManagementOptions{}, kernel.Fail(kernel.ErrUsageInvalid, "--limit must be a non-negative number")
		}
	}
	return revision, dataset.ManagementOptions{Limit: limit, Continuation: r.URL.Query().Get("continuation")}, nil
}

func (h *handler) datasetShow(w http.ResponseWriter, r *http.Request) {
	revision, options, err := managementOptions(r)
	if err != nil {
		writeError(w, err)
		return
	}
	h.withResources(w, r, "dataset-show", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		cat, err := openAuthorizedDatasets(ctx, caller, resources, "", "inventory.read")
		if err != nil {
			return nil, err
		}
		return dataset.ManagementView(cat.DumpState(), r.PathValue("dataset"), revision, options)
	})
}

func (h *handler) datasetDefine(w http.ResponseWriter, r *http.Request) {
	var request datasetRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	r = requestWithDataset(r, request.Dataset)
	h.withResources(w, r, "dataset-define", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		cat, err := openAuthorizedDatasets(ctx, caller, resources, request.Dataset, "dataset.manage")
		if err != nil {
			return nil, err
		}
		if len(request.Sources) == 0 {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "typed Workspace payload must contain sources")
		}
		if err := authorizePublicationSources(ctx, caller, resources, request.Dataset, request.Sources); err != nil {
			return nil, err
		}
		return resources.runOperation(ctx, caller, "dataset-define", func() (any, error) {
			return publishDataset(ctx, caller, resources, cat, request)
		})
	})
}

func publishDataset(ctx context.Context, caller identity.HTTPIdentity, resources Resources, cat dataset.Service, request datasetRequest) (any, error) {
	// The embedding application's before-hook may have revoked the caller.
	// Recheck before any source inspection or portable-recipe side effect.
	if err := authorizePublication(ctx, caller, resources, request.Dataset, request.Sources); err != nil {
		return nil, err
	}
	if accepted, ok := dataset.ReplayedDefinition(cat, request.Dataset, request.Revision, request.Sources, request.Metadata); ok {
		return definedDataset(accepted, dataset.DefineReplayed, nil), nil
	}
	// Validate before an optional independent portable-recipe write. Its
	// resulting source version is frozen again before the serving switch.
	if _, err := cat.PrepareDataset(request.Dataset, request.Revision, request.Sources); err != nil {
		return nil, err
	}
	definition := dataset.Dataset{SetID: request.Dataset, Revision: request.Revision, Sources: request.Sources, Metadata: maps.Clone(request.Metadata)}
	var receipt map[string]any
	if resources.BeforePublish != nil {
		var err error
		definition, receipt, err = resources.BeforePublish(ctx, caller, cat, definition)
		if err != nil {
			return nil, err
		}
		if definition.SetID != request.Dataset || definition.Revision != request.Revision {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "publication preparation changed the Dataset identity")
		}
	}
	if err := authorizePublication(ctx, caller, resources, definition.SetID, definition.Sources); err != nil {
		return nil, err
	}
	// Keep the caller's actual selection separate from the commits produced by
	// freezing a selector. A main-scoped grant must be rechecked as main, while
	// an explicitly supplied commit cannot borrow that selector's authority.
	selectedSources := clonePublicationSources(definition.Sources)
	metadata := maps.Clone(definition.Metadata)
	definition, err := cat.PrepareDataset(definition.SetID, definition.Revision, definition.Sources)
	if err != nil {
		return nil, err
	}
	definition.Metadata = metadata
	frozenSources := clonePublicationSources(definition.Sources)
	if resources.PreparePublication != nil {
		prepared, err := resources.PreparePublication(ctx, caller, definition)
		if err != nil {
			return nil, err
		}
		if prepared.SetID != definition.SetID || prepared.Revision != definition.Revision {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "capability preparation changed the Dataset identity")
		}
		definition.Sources, definition.Dependencies = prepared.Sources, prepared.Dependencies
	}
	if !reflect.DeepEqual(definition.Sources, frozenSources) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "capability preparation changed frozen publication sources")
	}
	if err := authorizePublication(ctx, caller, resources, definition.SetID, selectedSources); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	accepted, err := cat.PublishDataset(definition)
	if err != nil {
		return nil, err
	}
	return definedDataset(accepted, dataset.DefineApplied, receipt), nil
}

func definedDataset(accepted dataset.Dataset, disposition string, receipt map[string]any) map[string]any {
	out := map[string]any{"setId": accepted.SetID, "revision": accepted.Revision, "sources": accepted.Sources, "disposition": disposition}
	if len(accepted.Metadata) > 0 {
		out["metadata"] = maps.Clone(accepted.Metadata)
	}
	for _, key := range []string{"recipeFile", "recipeRepository", "recipeCommit", "recipeLocation", "recipeSkipped"} {
		if value, ok := receipt[key]; ok {
			out[key] = value
		}
	}
	return out
}

func authorizePublication(ctx context.Context, caller identity.HTTPIdentity, resources Resources, id string, sources []dataset.DatasetSource) error {
	if err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: "dataset.manage", Dataset: id}}); err != nil {
		return err
	}
	return authorizePublicationSources(ctx, caller, resources, id, sources)
}

func authorizePublicationSources(ctx context.Context, caller identity.HTTPIdentity, resources Resources, id string, sources []dataset.DatasetSource) error {
	for _, source := range sources {
		ref := snapshot.RefOrDefault(source.Selector)
		if source.Commit != "" {
			ref = string(source.Commit)
		}
		if err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: access.RepositoryDatasetPublish, Repo: string(source.Repository), Dataset: id, Ref: ref}}); err != nil {
			return err
		}
	}
	return nil
}

func clonePublicationSources(sources []dataset.DatasetSource) []dataset.DatasetSource {
	cloned := slices.Clone(sources)
	for i := range cloned {
		if cloned[i].Path != nil {
			value := *cloned[i].Path
			cloned[i].Path = &value
		}
	}
	return cloned
}

func (h *handler) datasetRetire(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyRequest(w, r) {
		return
	}
	h.withResources(w, r, "dataset-retire", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		id := r.PathValue("dataset")
		cat, err := openAuthorizedDatasets(ctx, caller, resources, id, "dataset.manage")
		if err != nil {
			return nil, err
		}
		return resources.runOperation(ctx, caller, "dataset-retire", func() (any, error) {
			if err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: "dataset.manage", Dataset: id}}); err != nil {
				return nil, err
			}
			if err := cat.RetireDataset(id); err != nil {
				return nil, err
			}
			return map[string]any{"dataset": id, "retired": true}, nil
		})
	})
}

func datasetResolveRevision(request datasetResolveRequest) (*int, error) {
	if len(request.Revision) == 0 {
		return nil, nil
	}
	if len(request.Pin) != 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "revision and pin are mutually exclusive")
	}
	var revision int
	if err := json.Unmarshal(request.Revision, &revision); err != nil || revision <= 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "revision must be a positive integer")
	}
	return &revision, nil
}

func resolveNamedDataset(cat dataset.Service, id string, request datasetResolveRequest, revision *int) (dataset.ResolvedDataset, error) {
	if revision != nil {
		definition, err := cat.DatasetVersion(id, *revision)
		if err != nil {
			return dataset.ResolvedDataset{}, err
		}
		pin := dataset.ResolvedDataset{SetID: definition.SetID, Revision: definition.Revision, Repositories: map[kernel.RepositoryID]kernel.CommitID{}}
		for _, source := range definition.Sources {
			if source.Commit == "" {
				return dataset.ResolvedDataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "published Dataset source has no frozen commit")
			}
			pin.Repositories[source.Repository] = source.Commit
		}
		return cat.ReplayPublished(pin)
	}
	if len(request.Pin) == 0 {
		return cat.ResolveDataset(id)
	}
	var pin dataset.ResolvedDataset
	if err := dataset.DecodeJSON(request.Pin, &pin); err != nil {
		return dataset.ResolvedDataset{}, kernel.Fail(kernel.ErrUsageInvalid, "pin must be a ResolvedDataset JSON object")
	}
	if pin.SetID != "" && pin.SetID != id {
		return dataset.ResolvedDataset{}, kernel.Fail(kernel.ErrUsageInvalid, "pin names a different dataset")
	}
	pin.SetID = id
	return cat.ReplayPublished(pin)
}

func (h *handler) datasetResolve(w http.ResponseWriter, r *http.Request) {
	h.datasetResolveOrCheck(w, r, false)
}

func (h *handler) datasetCheck(w http.ResponseWriter, r *http.Request) {
	h.datasetResolveOrCheck(w, r, true)
}

func (h *handler) datasetResolveOrCheck(w http.ResponseWriter, r *http.Request, check bool) {
	var request datasetResolveRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	if check && len(request.Revision) != 0 {
		writeError(w, kernel.Fail(kernel.ErrUsageInvalid, "revision is only valid for Dataset resolve"))
		return
	}
	revision, err := datasetResolveRevision(request)
	if err != nil {
		writeError(w, err)
		return
	}
	if check && request.InventoryDiscovery {
		writeError(w, kernel.Fail(kernel.ErrUsageInvalid, "inventoryDiscovery is only valid for ResolveDataset and SEARCH"))
		return
	}
	operation := "pin"
	if check {
		operation = "pin-check"
	}
	h.withResources(w, r, operation, func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		id := r.PathValue("dataset")
		if err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: "dataset.resolve", Dataset: id}, InventoryDiscovery: request.InventoryDiscovery}); err != nil {
			return nil, err
		}
		cat, err := openDatasets(resources)
		if err != nil {
			return nil, err
		}
		if revision == nil && resources.EnsureDataset != nil {
			if err := resources.EnsureDataset(ctx, caller, cat, id); err != nil {
				return nil, err
			}
		}
		pin, err := resolveNamedDataset(cat, id, request, revision)
		if err != nil {
			return nil, err
		}
		if check {
			if err := authorizeCheckedFiles(ctx, caller, resources, pin); err != nil {
				return nil, err
			}
			return cat.CheckResolved(pin), nil
		}
		return pin, nil
	})
}

// The pin has already been reconstructed from an accepted release. Check may
// probe its selected bodies, so all fixed sources require current consumption
// permission before any provider is touched. A Dataset management or source
// publication grant cannot substitute for file.read here.
func authorizeCheckedFiles(ctx context.Context, caller identity.HTTPIdentity, resources Resources, pin dataset.ResolvedDataset) error {
	for repository, commit := range pin.Repositories {
		request := Authorization{Caller: caller, Query: access.Query{Action: "file.read", Dataset: pin.SetID, Repo: string(repository), Ref: string(commit)}, Pin: &pin}
		if err := resources.authorize(ctx, request); err != nil {
			return err
		}
	}
	return nil
}

func authorizeDefinition(ctx context.Context, caller identity.HTTPIdentity, resources Resources, action string, definition *dataset.Dataset, pin *dataset.ResolvedDataset) error {
	request := Authorization{Caller: caller, Query: access.Query{Action: action}, Definition: definition, Pin: pin}
	err := resources.authorize(ctx, request)
	if err == nil || kernel.CodeOf(err) != kernel.ErrForbidden {
		return err
	}
	// An inline label never selects a named grant. Complete member grants
	// may authorize this recipe without creating a second Dataset authority.
	seen := map[kernel.RepositoryID]bool{}
	for _, source := range definition.Sources {
		if seen[source.Repository] {
			continue
		}
		seen[source.Repository] = true
		request.Query.Repo = string(source.Repository)
		if err := resources.authorize(ctx, request); err != nil {
			return err
		}
	}
	if len(seen) == 0 {
		return err
	}
	return nil
}

func (h *handler) datasetResolveDefinition(w http.ResponseWriter, r *http.Request) {
	var request datasetRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	r = requestWithDataset(r, request.Dataset)
	h.withResources(w, r, "pin-source", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		definition := dataset.Dataset{SetID: request.Dataset, Revision: request.Revision, Sources: request.Sources, Metadata: maps.Clone(request.Metadata)}
		if err := authorizeDefinition(ctx, caller, resources, "dataset.resolve", &definition, nil); err != nil {
			return nil, err
		}
		cat, err := openDatasets(resources)
		if err != nil {
			return nil, err
		}
		pin, err := cat.ResolveDefinition(definition)
		if err != nil {
			return nil, err
		}
		if err := authorizeDefinition(ctx, caller, resources, "file.read", &definition, &pin); err != nil {
			return nil, err
		}
		return pin, nil
	})
}
