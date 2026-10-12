package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

func (h *Handler) registerDatasetRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /catalog/v1/catalogs", h.catalogList)
	mux.HandleFunc("GET /catalog/v1/catalogs/{catalog}/repositories", h.catalogRepositories)
	mux.HandleFunc("GET /catalog/v1/catalogs/{catalog}/datasets", h.datasetList)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/datasets", h.datasetDefine)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/datasets:resolve", h.datasetResolveDefinition)
	mux.HandleFunc("GET /catalog/v1/catalogs/{catalog}/datasets/{dataset}", h.datasetShow)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/datasets/{dataset}/retire", h.datasetRetire)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/datasets/{dataset}/resolve", h.datasetResolve)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/datasets/{dataset}/check", h.datasetCheck)
}

type datasetRequest struct {
	Dataset  string                       `json:"dataset"`
	Revision int                          `json:"revision"`
	Sources  []dataset.KnowledgeSetSource `json:"sources"`
}

type datasetResolveRequest struct {
	Pin              json.RawMessage `json:"pin,omitempty"`
	CatalogDiscovery bool            `json:"catalogDiscovery,omitempty"`
}

func (h *Handler) catalogList(w http.ResponseWriter, r *http.Request) {
	h.withResources(w, r, "catalog-list", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		if resources.Catalogs == nil {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Catalog provider is required")
		}
		visible := []map[string]any{}
		for _, id := range resources.Catalogs.CatalogIDs() {
			err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: "catalog.read", Catalog: id}})
			if kernel.CodeOf(err) == kernel.ErrForbidden {
				continue
			}
			if err != nil {
				return nil, err
			}
			visible = append(visible, map[string]any{"id": id})
		}
		return map[string]any{"catalogs": visible}, nil
	})
}

func openAuthorizedCatalog(ctx context.Context, caller identity.HTTPIdentity, resources Resources, catalogID, datasetID, action string) (*dataset.Catalog, error) {
	if strings.TrimSpace(catalogID) == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "Catalog identity is required")
	}
	if err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: action, Catalog: catalogID, Dataset: datasetID}}); err != nil {
		return nil, err
	}
	return openCatalog(resources, catalogID)
}

func openCatalog(resources Resources, catalogID string) (*dataset.Catalog, error) {
	if resources.Catalogs == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Catalog provider is required")
	}
	cat, err := resources.Catalogs.OpenCatalog(catalogID)
	if err == nil && cat == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Catalog provider returned no Catalog")
	}
	if err == nil && catalogID != "" && cat.DumpState().CatalogID != catalogID {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Catalog provider returned a different Catalog authority")
	}
	return cat, err
}

func (h *Handler) datasetList(w http.ResponseWriter, r *http.Request) {
	h.withResources(w, r, "dataset-list", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		cat, err := openAuthorizedCatalog(ctx, caller, resources, r.PathValue("catalog"), "", "catalog.read")
		if err != nil {
			return nil, err
		}
		state := dataset.NormalizeCatalogState(cat.DumpState())
		return map[string]any{"catalogId": state.CatalogID, "datasets": dataset.PublicKnowledgeSets(state.KnowledgeSets)}, nil
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

func (h *Handler) datasetShow(w http.ResponseWriter, r *http.Request) {
	revision, options, err := managementOptions(r)
	if err != nil {
		writeError(w, err)
		return
	}
	h.withResources(w, r, "dataset-show", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		cat, err := openAuthorizedCatalog(ctx, caller, resources, r.PathValue("catalog"), "", "catalog.read")
		if err != nil {
			return nil, err
		}
		return dataset.ManagementView(cat.DumpState(), r.PathValue("dataset"), revision, options)
	})
}

func (h *Handler) datasetDefine(w http.ResponseWriter, r *http.Request) {
	var request datasetRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	r = requestWithDataset(r, request.Dataset)
	h.withResources(w, r, "dataset-define", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		cat, err := openAuthorizedCatalog(ctx, caller, resources, r.PathValue("catalog"), request.Dataset, "dataset.manage")
		if err != nil {
			return nil, err
		}
		if len(request.Sources) == 0 {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "typed Workspace payload must contain sources")
		}
		return resources.runOperation(ctx, caller, "dataset-define", func() (any, error) {
			return publishDataset(ctx, caller, resources, cat, request, r.PathValue("catalog"))
		})
	})
}

func publishDataset(ctx context.Context, caller identity.HTTPIdentity, resources Resources, cat *dataset.Catalog, request datasetRequest, catalogID string) (any, error) {
	// The embedding application's before-hook may have revoked the caller.
	// Recheck before any source inspection or portable-recipe side effect.
	if err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: "dataset.manage", Catalog: catalogID, Dataset: request.Dataset}}); err != nil {
		return nil, err
	}
	// Validate before an optional independent portable-recipe write. Its
	// resulting source version is frozen again before the serving switch.
	if _, err := cat.PrepareKnowledgeSet(request.Dataset, request.Revision, request.Sources); err != nil {
		return nil, err
	}
	definition := dataset.KnowledgeSet{SetID: request.Dataset, Revision: request.Revision, Sources: request.Sources}
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
	if err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: "dataset.manage", Catalog: catalogID, Dataset: request.Dataset}}); err != nil {
		return nil, err
	}
	definition, err := cat.PrepareKnowledgeSet(definition.SetID, definition.Revision, definition.Sources)
	if err != nil {
		return nil, err
	}
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
	for _, key := range []string{"recipeFile", "recipeRepository", "recipeCommit", "recipeLocation", "recipeSkipped"} {
		if value, ok := receipt[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (h *Handler) datasetRetire(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyRequest(w, r) {
		return
	}
	h.withResources(w, r, "dataset-retire", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		id := r.PathValue("dataset")
		cat, err := openAuthorizedCatalog(ctx, caller, resources, r.PathValue("catalog"), id, "dataset.manage")
		if err != nil {
			return nil, err
		}
		return resources.runOperation(ctx, caller, "dataset-retire", func() (any, error) {
			if err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: "dataset.manage", Catalog: r.PathValue("catalog"), Dataset: id}}); err != nil {
				return nil, err
			}
			if err := cat.RetireKnowledgeSet(id); err != nil {
				return nil, err
			}
			return map[string]any{"dataset": id, "retired": true}, nil
		})
	})
}

func resolveNamedDataset(cat *dataset.Catalog, id string, request datasetResolveRequest) (dataset.ResolvedKnowledgeSet, error) {
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

func (h *Handler) datasetResolve(w http.ResponseWriter, r *http.Request) {
	h.datasetResolveOrCheck(w, r, false)
}

func (h *Handler) datasetCheck(w http.ResponseWriter, r *http.Request) {
	h.datasetResolveOrCheck(w, r, true)
}

func (h *Handler) datasetResolveOrCheck(w http.ResponseWriter, r *http.Request, check bool) {
	var request datasetResolveRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	if check && request.CatalogDiscovery {
		writeError(w, kernel.Fail(kernel.ErrUsageInvalid, "catalogDiscovery is only valid for ResolveKnowledgeSet and SEARCH"))
		return
	}
	operation := "pin"
	if check {
		operation = "pin-check"
	}
	h.withResources(w, r, operation, func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		catalogID, id := r.PathValue("catalog"), r.PathValue("dataset")
		if err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: "dataset.resolve", Catalog: catalogID, Dataset: id}, CatalogDiscovery: request.CatalogDiscovery}); err != nil {
			return nil, err
		}
		cat, err := openCatalog(resources, catalogID)
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

func authorizeDefinition(ctx context.Context, caller identity.HTTPIdentity, resources Resources, catalogID, action string, definition *dataset.KnowledgeSet, pin *dataset.ResolvedKnowledgeSet) error {
	request := Authorization{Caller: caller, Query: access.Query{Action: action, Catalog: catalogID}, Definition: definition, Pin: pin}
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

func (h *Handler) datasetResolveDefinition(w http.ResponseWriter, r *http.Request) {
	var request datasetRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	r = requestWithDataset(r, request.Dataset)
	h.withResources(w, r, "pin-source", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		catalogID := r.PathValue("catalog")
		definition := dataset.KnowledgeSet{SetID: request.Dataset, Revision: request.Revision, Sources: request.Sources}
		if err := authorizeDefinition(ctx, caller, resources, catalogID, "dataset.resolve", &definition, nil); err != nil {
			return nil, err
		}
		cat, err := openCatalog(resources, catalogID)
		if err != nil {
			return nil, err
		}
		pin, err := cat.ResolveDefinition(definition)
		if err != nil {
			return nil, err
		}
		if err := authorizeDefinition(ctx, caller, resources, catalogID, "file.read", &definition, &pin); err != nil {
			return nil, err
		}
		return pin, nil
	})
}
