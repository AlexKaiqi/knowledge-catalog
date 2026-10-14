package invocation

import (
	discoveryservice "kc/discovery/service"
	objectserving "kc/liveaccess"

	index "kc/discovery/projection"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"

	retrieval "kc/discovery"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func VerbSearch(cx *Invocation) (any, error) {
	// The HTTP server passes its request-time embedding provider through the
	// internal _embedder stamp (like _search-request). Model providers live
	// only in the server; the plain CLI leaves the stamp unset and semantic
	// recall fails closed there.
	embedder, _ := cx.Flags["_embedder"].(retrieval.Embedder)
	if ServingWorkspace(cx.Flags) {
		return SearchWorkspace(cx)
	}
	repositoryID, commitID, err := PinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	req, err := SearchRequestFromFlags(cx.Flags)
	if err != nil {
		return nil, err
	}
	out, err := (discoveryservice.SearchExecutor{
		Repositories: cx.WS.Reader,
		Projection:   cx.WS.Index,
		Embedder:     embedder,
	}).Execute(cx.Context, discoveryservice.SearchRequest{
		Repository: repositoryID, Commit: commitID, Query: req,
	})
	if err != nil {
		return nil, err
	}
	return DeliverSearchResult(cx.Home, cx.Flags, out)
}

func VerbDescribeIndex(cx *Invocation) (any, error) {
	repoID, commitID, err := PinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	return (discoveryservice.ProjectionDescribeExecutor{
		Repositories: cx.WS.Reader,
		Projection:   cx.WS.Index,
	}).Execute(cx.Context, discoveryservice.ProjectionDescribeRequest{
		Repository: repoID,
		Commit:     commitID,
	})
}

func VerbIndexSync(cx *Invocation) (any, error) {
	repositoryID, commitID, err := PinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	result, err := (discoveryservice.ProjectionSyncExecutor{
		Repositories: cx.WS.Reader,
		Projection:   cx.WS.Index,
		Controller:   cx.WS.Plugins,
		Observe: func() func() {
			return ObserveProjectionExecution(cx)
		},
	}).Execute(cx.Context, discoveryservice.ProjectionSyncRequest{
		Repository: repositoryID,
		Commit:     commitID,
		State:      cx.State,
		StateRequest: func() (objectmodel.RequestContext, error) {
			return stateRequestContextFrom(cx)
		},
	})
	if err != nil {
		return nil, err
	}
	if result.State == nil {
		return result.Snapshot, nil
	}
	return map[string]any{"snapshot": result.Snapshot, "state": *result.State}, nil
}

func VerbIndexNotify(cx *Invocation) (any, error) {
	notice, err := ChangeNoticeFromFlags(cx.Flags)
	if err != nil {
		return nil, err
	}
	if _, err := RequireRepo(cx.WS, string(notice.Repository)); err != nil {
		return nil, err
	}
	if cx.WS.Plugins == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection controller is not configured")
	}
	lookup, err := ResourceLookup(cx)
	if err != nil {
		return nil, err
	}
	request, err := stateRequestContextFrom(cx)
	if err != nil {
		return nil, err
	}
	return (discoveryservice.ProjectionNoticeExecutor{
		Repositories: cx.WS.Reader,
		Projection:   cx.WS.Index,
		Controller:   cx.WS.Plugins,
	}).Execute(cx.Context, discoveryservice.ProjectionNoticeRequest{
		Notice:  notice,
		State:   lookup,
		Request: request,
	})
}

func ChangeNoticeFromFlags(flags map[string]invoke.FlagValue) (index.ChangeNotice, error) {
	repo, err := invoke.RequireFlag(flags, "repo")
	if err != nil {
		return index.ChangeNotice{}, err
	}
	notice := index.ChangeNotice{
		Repository:     kernel.RepositoryID(repo),
		Ref:            invoke.FlagString(flags, "ref"),
		SourceRevision: invoke.FlagString(flags, "source-revision"),
	}
	if object := invoke.FlagString(flags, "object"); object != "" {
		kind := objectmodel.AddressKind(invoke.FlagString(flags, "kind"))
		if kind == "" {
			if invoke.FlagString(flags, "aspect") != "" {
				kind = objectmodel.KindAspect
			} else {
				kind = objectmodel.KindEntity
			}
		}
		notice.Address = &objectmodel.Address{
			Kind: kind, ObjectID: objectmodel.ObjectID(object), AspectName: invoke.FlagString(flags, "aspect"),
		}
	}
	return notice, index.ValidateChangeNotice(notice)
}

// VerbDescribeAccess reports one logical AccessSpec per pinned member, or the
// single AccessSpec of one Repository's published basis.
func VerbDescribeAccess(cx *Invocation) (any, error) {
	cat, err := PickDatasets(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	var resolved dataset.ResolvedDataset
	if repository := invoke.FlagString(cx.Flags, "repo"); repository != "" && invoke.FlagString(cx.Flags, "dataset") == "" {
		resolved, err = cat.ResolveDefinition(dataset.Dataset{
			Revision: 1,
			Sources:  []dataset.DatasetSource{{Repository: kernel.RepositoryID(repository), Selector: snapshot.DefaultRef}},
		})
	} else {
		setID, workspaceErr := cx.SetID()
		if workspaceErr != nil {
			return nil, workspaceErr
		}
		resolved, err = ResolveOrReplay(cx.WS, cx.Home, cat, setID, cx.Flags)
	}
	if err != nil {
		return nil, err
	}
	pin := DatasetPin(resolved)
	if err := RequireCompleteWorkspaceRead(cx.Home, cx.Flags, pin, ""); err != nil {
		return nil, err
	}
	result, err := (discoveryservice.AccessDescribeExecutor{
		Repositories: cx.WS.Reader.Lookup(cat.Require),
		Plugins:      cx.WS.Plugins,
	}).Execute(cx.Context, discoveryservice.AccessDescribeRequest{Pin: pin})
	if err != nil {
		return nil, err
	}
	plugins, err := cx.WS.LiveAccess.Overview(pin)
	if err != nil {
		return nil, err
	}
	result.Plugins = append(result.Plugins, plugins...)
	return result, nil
}

// ResourceLookup uses an explicit component port or the Repository-selected
// LiveAccess registry. A static read needs no external lookup.
func ResourceLookup(cx *Invocation) (objectserving.StateLookup, error) {
	if cx.State != nil {
		return cx.State, nil
	}
	if cx.WS != nil && cx.WS.LiveAccess != nil {
		return cx.WS.LiveAccess, nil
	}
	return nil, nil
}
