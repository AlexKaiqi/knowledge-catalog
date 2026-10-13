package service

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"strconv"
	"strings"

	"kc/identity"
	"kc/kernel"
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
	Dataset  string                       `json:"dataset"`
	Revision int                          `json:"revision"`
	Sources  []dataset.KnowledgeSetSource `json:"sources"`
	Metadata datasetMetadata              `json:"metadata,omitempty"`
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
		return map[string]any{"datasets": dataset.PublicKnowledgeSets(state.KnowledgeSets)}, nil
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
		return resources.runOperation(ctx, caller, "dataset-define", func() (any, error) {
			return publishDataset(ctx, caller, resources, cat, request)
		})
	})
}

func publishDataset(ctx context.Context, caller identity.HTTPIdentity, resources Resources, cat dataset.Service, request datasetRequest) (any, error) {
	// The embedding application's before-hook may have revoked the caller.
	// Recheck before any source inspection or portable-recipe side effect.
	if err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: "dataset.manage", Dataset: request.Dataset}}); err != nil {
		return nil, err
	}
	// Validate before an optional independent portable-recipe write. Its
	// resulting source version is frozen again before the serving switch.
	if _, err := cat.PrepareKnowledgeSet(request.Dataset, request.Revision, request.Sources); err != nil {
		return nil, err
	}
	definition := dataset.KnowledgeSet{SetID: request.Dataset, Revision: request.Revision, Sources: request.Sources, Metadata: maps.Clone(request.Metadata)}
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
	if err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: "dataset.manage", Dataset: request.Dataset}}); err != nil {
		return nil, err
	}
	metadata := maps.Clone(definition.Metadata)
	definition, err := cat.PrepareKnowledgeSet(definition.SetID, definition.Revision, definition.Sources)
	if err != nil {
		return nil, err
	}
	definition.Metadata = metadata
	if resources.PreparePublication != nil {
		if err := resources.PreparePublication(ctx, caller, definition); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	accepted, err := cat.PublishKnowledgeSet(definition)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"setId": accepted.SetID, "revision": accepted.Revision, "sources": accepted.Sources}
	if len(accepted.Metadata) > 0 {
		out["metadata"] = maps.Clone(accepted.Metadata)
	}
	for _, key := range []string{"recipeFile", "recipeRepository", "recipeCommit", "recipeLocation", "recipeSkipped"} {
		if value, ok := receipt[key]; ok {
			out[key] = value
		}
	}
	return out, nil
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
			if err := cat.RetireKnowledgeSet(id); err != nil {
				return nil, err
			}
			return map[string]any{"dataset": id, "retired": true}, nil
		})
	})
}

func resolveNamedDataset(cat dataset.Service, id string, request datasetResolveRequest) (dataset.ResolvedKnowledgeSet, error) {
	if len(request.Pin) == 0 {
		return cat.ResolveKnowledgeSet(id)
	}
	var pin dataset.ResolvedKnowledgeSet
	if err := dataset.DecodeJSON(request.Pin, &pin); err != nil {
		return dataset.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid, "pin must be a ResolvedKnowledgeSet JSON object")
	}
	if pin.SetID != "" && pin.SetID != id {
		return dataset.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid, "pin names a different dataset")
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
	if check && request.InventoryDiscovery {
		writeError(w, kernel.Fail(kernel.ErrUsageInvalid, "inventoryDiscovery is only valid for ResolveKnowledgeSet and SEARCH"))
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
		if resources.EnsureDataset != nil {
			if err := resources.EnsureDataset(ctx, caller, cat, id); err != nil {
				return nil, err
			}
		}
		pin, err := resolveNamedDataset(cat, id, request)
		if err != nil {
			return nil, err
		}
		if check {
			return cat.CheckResolved(pin), nil
		}
		return pin, nil
	})
}

func authorizeDefinition(ctx context.Context, caller identity.HTTPIdentity, resources Resources, action string, definition *dataset.KnowledgeSet, pin *dataset.ResolvedKnowledgeSet) error {
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
		definition := dataset.KnowledgeSet{SetID: request.Dataset, Revision: request.Revision, Sources: request.Sources, Metadata: maps.Clone(request.Metadata)}
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
