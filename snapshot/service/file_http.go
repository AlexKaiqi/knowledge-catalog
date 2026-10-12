package service

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

func (h *Handler) registerFileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /dataset-files/v1/mounts:list", h.fileMounts)
	mux.HandleFunc("POST /dataset-files/v1/tree:list", h.fileDirectory)
	mux.HandleFunc("POST /dataset-files/v1/file:read", h.fileRead)
}

func (h *Handler) fileMounts(w http.ResponseWriter, r *http.Request) {
	var request FileMountsRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	r = requestWithDataset(r, request.Dataset)
	r.SetPathValue("catalog", request.Catalog)
	h.withResources(w, r, "file-mounts", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		view, err := resources.OpenFiles(ctx, caller, request.FileCoordinate, false)
		if err != nil {
			return nil, err
		}
		return FileMountsResponse{Pin: view.FixedPin(), Mounts: view.VisibleMounts()}, nil
	})
}

func (h *Handler) fileDirectory(w http.ResponseWriter, r *http.Request) {
	var request FileDirectoryRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	r = requestWithDataset(r, request.Dataset)
	r.SetPathValue("catalog", request.Catalog)
	h.withResources(w, r, "file-list", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		view, err := resources.OpenFiles(ctx, caller, request.FileCoordinate, true)
		if err != nil {
			return nil, err
		}
		return view.List(request)
	})
}

func (h *Handler) fileRead(w http.ResponseWriter, r *http.Request) {
	var request FileReadRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	r = requestWithDataset(r, request.Dataset)
	r.SetPathValue("catalog", request.Catalog)
	h.withResources(w, r, "file-read", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		view, err := resources.OpenFiles(ctx, caller, request.FileCoordinate, true)
		if err != nil {
			return nil, err
		}
		return view.Read(request)
	})
}

// OpenFiles reconstructs a scope from the accepted release and rechecks current
// authorization. A caller pin never supplies bytes, membership or permission.
func (resources Resources) OpenFiles(ctx context.Context, caller identity.HTTPIdentity, coordinate FileCoordinate, requirePin bool) (*FileView, error) {
	if resources.Catalogs == nil || resources.Sources == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "file Dataset authorities are required")
	}
	if strings.TrimSpace(coordinate.Dataset) == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "workspace is required")
	}
	if requirePin && len(coordinate.Pin) == 0 {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "a fixed ResolvedKnowledgeSet pin is required")
	}
	// Check the requested scope before opening its authority: an unauthorized
	// caller must not distinguish a hidden Catalog from a missing Catalog.
	query := access.Query{Principal: caller.Principal, Action: "dataset.resolve", Catalog: coordinate.Catalog, Dataset: coordinate.Dataset}
	if err := resources.authorize(ctx, Authorization{Caller: caller, Query: query}); err != nil {
		return nil, err
	}
	cat, err := openCatalog(resources, coordinate.Catalog)
	if err != nil {
		return nil, err
	}
	catalogID := cat.DumpState().CatalogID
	// The provider may apply an embedding application's default selection or
	// canonicalization. Its actual returned authority must also be allowed.
	query.Catalog = catalogID
	if err := resources.authorize(ctx, Authorization{Caller: caller, Query: query}); err != nil {
		return nil, err
	}
	if resources.EnsureDataset != nil {
		if err := resources.EnsureDataset(ctx, caller, cat, coordinate.Dataset); err != nil {
			return nil, err
		}
	}
	definition, err := cat.Set(coordinate.Dataset)
	if err != nil {
		return nil, err
	}
	var pin dataset.ResolvedKnowledgeSet
	if len(coordinate.Pin) > 0 {
		if err := kernel.UnmarshalJSON(coordinate.Pin, &pin); err != nil {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "invalid fixed Dataset pin: %v", err)
		}
		if pin.SetID != "" && pin.SetID != coordinate.Dataset {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "pin names a different dataset")
		}
		pin.SetID = coordinate.Dataset
		pin, err = cat.ReplayPublished(pin)
		if err == nil {
			definition, err = cat.DatasetVersion(coordinate.Dataset, pin.Revision)
		}
	} else {
		pin, err = cat.ResolveKnowledgeSet(coordinate.Dataset)
	}
	if err != nil {
		return nil, err
	}
	if coordinate.View != "" && coordinate.View != "repository" {
		if resources.OpenView == nil {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "the requested external file view is not configured")
		}
		view, err := resources.OpenView(ctx, caller, coordinate, cat, definition, pin)
		if err != nil {
			return nil, err
		}
		if view == nil {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "external file view returned no view")
		}
		return view, nil
	}
	mounts, err := dataset.ListVirtualMountsAt(definition, pin)
	if err != nil {
		return nil, err
	}
	mayRead := func(repository kernel.RepositoryID) (bool, error) {
		q := access.Query{Principal: caller.Principal, Action: "file.read", Catalog: catalogID, Dataset: coordinate.Dataset, Repo: string(repository)}
		err := resources.authorize(ctx, Authorization{Caller: caller, Query: q, Pin: &pin})
		if kernel.CodeOf(err) == kernel.ErrForbidden {
			return false, nil
		}
		return err == nil, err
	}
	visible := make([]dataset.VirtualMount, 0, len(mounts))
	for _, mount := range mounts {
		ok, err := mayRead(mount.Repository)
		if err != nil {
			return nil, err
		}
		if ok {
			visible = append(visible, mount)
		}
	}
	sort.Slice(visible, func(i, j int) bool { return visible[i].Path < visible[j].Path })
	items := make([]dataset.DatasetItem, 0, len(pin.Items))
	for _, item := range pin.Items {
		if item.Kind != dataset.DatasetItemFile {
			continue
		}
		ok, err := mayRead(item.Repository)
		if err != nil {
			return nil, err
		}
		if ok {
			items = append(items, item)
		}
	}
	options := ViewOptions{Catalog: catalogID, Pin: pin, Mounts: visible, Items: items, Sources: resources.Sources, Delivered: true}
	if resources.RecordRead != nil {
		options.Record = func(evidence FileReadEvidence) error { return resources.RecordRead(ctx, caller, evidence) }
	}
	return NewFileView(options), nil
}
