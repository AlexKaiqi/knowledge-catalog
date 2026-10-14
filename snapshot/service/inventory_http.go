package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

func (h *handler) registerInventoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /snapshot/v1/inventory", h.inventoryShow)
	mux.HandleFunc("GET /snapshot/v1/audit", h.registryAudit)
	mux.HandleFunc("POST /snapshot/v1/repositories", h.repositoryAttach)
	mux.HandleFunc("DELETE /snapshot/v1/repositories/{repository}", h.repositoryDetach)
}

func (h *handler) inventoryShow(w http.ResponseWriter, r *http.Request) {
	h.withResources(w, r, "inventory-show", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		cat, err := openAuthorizedDatasets(ctx, caller, resources, "", "inventory.read")
		if err != nil {
			return nil, err
		}
		return resources.runOperation(ctx, caller, "inventory-show", func() (any, error) {
			if err := resources.authorize(ctx, Authorization{Caller: caller, Query: access.Query{Action: "inventory.read"}}); err != nil {
				return nil, err
			}
			state := dataset.NormalizeState(cat.DumpState())
			repositories, err := repositoryInventory(ctx, caller, resources, state.Repositories)
			if err != nil {
				return nil, err
			}
			view := map[string]any{"repositories": repositories, "datasets": dataset.PublicKnowledgeSets(state.KnowledgeSets)}
			if resources.DiscoveryWorkspaceID != "" {
				view["discoveryWorkspaceId"] = resources.DiscoveryWorkspaceID
			}
			return view, nil
		})
	})
}

func auditLimit(r *http.Request) (int, error) {
	if r.URL.Query().Get("continuation") != "" {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "registry audit does not provide continuation pages")
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 || parsed > 200 {
			return 0, kernel.Fail(kernel.ErrUsageInvalid, "audit limit must be a non-negative integer no greater than 200")
		}
		if parsed != 0 {
			limit = parsed
		}
	}
	return limit, nil
}

func (h *handler) registryAudit(w http.ResponseWriter, r *http.Request) {
	limit, err := auditLimit(r)
	if err != nil {
		writeError(w, err)
		return
	}
	h.withResources(w, r, "registry-audit", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		cat, err := openAuthorizedDatasets(ctx, caller, resources, "", "registry.audit.read")
		if err != nil {
			return nil, err
		}
		history, err := cat.Log(dataset.LogQuery{Limit: limit})
		if err != nil {
			return nil, err
		}
		return map[string]any{"source": "registry", "authorityId": history.RepositoryID, "entries": history.Commits}, nil
	})
}

type repositoryAttachRequest struct {
	Repository string `json:"repository"`
}

func (h *handler) repositoryAttach(w http.ResponseWriter, r *http.Request) {
	var request repositoryAttachRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	h.repositoryMembership(w, r, "repository-attach", request.Repository, true)
}

func (h *handler) repositoryDetach(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyRequest(w, r) {
		return
	}
	h.repositoryMembership(w, r, "detach", r.PathValue("repository"), false)
}

func (h *handler) repositoryMembership(w http.ResponseWriter, r *http.Request, operation, id string, attach bool) {
	if strings.TrimSpace(id) == "" {
		writeError(w, kernel.Fail(kernel.ErrUsageInvalid, "repository identity is required"))
		return
	}
	r = r.Clone(r.Context())
	r.SetPathValue("repository", id)
	h.withResources(w, r, operation, func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		query := access.Query{Action: "repositories.manage", Repo: id}
		if err := resources.authorize(ctx, Authorization{Caller: caller, Query: query}); err != nil {
			return nil, err
		}
		cat, err := openDatasets(resources)
		if err != nil {
			return nil, err
		}
		return resources.runOperation(ctx, caller, operation, func() (any, error) {
			if err := resources.authorize(ctx, Authorization{Caller: caller, Query: query}); err != nil {
				return nil, err
			}
			repository := kernel.RepositoryID(id)
			if !attach {
				if err := cat.UnregisterRepository(repository); err != nil {
					return nil, err
				}
				return map[string]any{"repositoryId": id, "detached": true}, nil
			}
			if resources.BeforeAttach != nil {
				if err := resources.BeforeAttach(ctx, caller, repository); err != nil {
					return nil, err
				}
			}
			if resources.Sources == nil {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Snapshot sources are required for registration")
			}
			source, ok := resources.Sources.Get(repository)
			if !ok {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Snapshot %s is unavailable", id)
			}
			if source.Archived() {
				return nil, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", id)
			}
			if err := cat.RegisterRepository(repository); err != nil {
				return nil, err
			}
			return map[string]any{"repositoryId": id}, nil
		})
	})
}
