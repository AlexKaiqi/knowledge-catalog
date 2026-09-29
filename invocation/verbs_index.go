package invocation

import (
	knowledgeserving "kc/knowledge/serving"

	"kc/catalog"
	"kc/index"
	"kc/invoke"
	"kc/kernel"
	knowledge "kc/knowledge"
	knowledgeapp "kc/knowledgeapp"
	"kc/retrieval"
	"kc/snapshot"
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
	out, err := (knowledgeapp.SearchExecutor{
		Repositories: cx.WS.Reader,
		Projection:   cx.WS.Index,
		Embedder:     embedder,
	}).Execute(cx.Context, knowledgeapp.SearchRequest{
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
	return (knowledgeapp.ProjectionDescribeExecutor{
		Repositories: cx.WS.Reader,
		Projection:   cx.WS.Index,
	}).Execute(cx.Context, knowledgeapp.ProjectionDescribeRequest{
		Repository: repoID,
		Commit:     commitID,
	})
}

func VerbIndexSync(cx *Invocation) (any, error) {
	repositoryID, commitID, err := PinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	result, err := (knowledgeapp.ProjectionSyncExecutor{
		Repositories: cx.WS.Reader,
		Projection:   cx.WS.Index,
		Controller:   cx.WS.Projection,
		Observe: func() func() {
			return ObserveProjectionExecution(cx)
		},
	}).Execute(cx.Context, knowledgeapp.ProjectionSyncRequest{
		Repository: repositoryID,
		Commit:     commitID,
		State:      cx.State,
		StateRequest: func() (knowledgeserving.RequestContext, error) {
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
	if cx.WS.Projection == nil {
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
	return (knowledgeapp.ProjectionNoticeExecutor{
		Repositories: cx.WS.Reader,
		Projection:   cx.WS.Index,
		Controller:   cx.WS.Projection,
	}).Execute(cx.Context, knowledgeapp.ProjectionNoticeRequest{
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
		kind := knowledge.AddressKind(invoke.FlagString(flags, "kind"))
		if kind == "" {
			if invoke.FlagString(flags, "aspect") != "" {
				kind = knowledge.KindAspect
			} else {
				kind = knowledge.KindEntity
			}
		}
		notice.Address = &knowledge.Address{
			Kind: kind, ObjectID: knowledge.ObjectID(object), AspectName: invoke.FlagString(flags, "aspect"),
		}
	}
	return notice, index.ValidateChangeNotice(notice)
}

// VerbDescribeAccess reports one logical AccessSpec per pinned member, or the
// single AccessSpec of one Repository's published basis.
func VerbDescribeAccess(cx *Invocation) (any, error) {
	cat, err := PickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	var resolved catalog.ResolvedKnowledgeSet
	if repository := invoke.FlagString(cx.Flags, "repo"); repository != "" && invoke.FlagString(cx.Flags, "dataset") == "" {
		resolved, err = cat.ResolveDefinition(catalog.KnowledgeSet{
			Revision: 1,
			Sources:  []catalog.KnowledgeSetSource{{Repository: kernel.RepositoryID(repository), Selector: snapshot.DefaultRef}},
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
	pin := KnowledgeSetPin(resolved)
	if err := RequireCompleteWorkspaceRead(cx.Home, cx.Flags, pin, ""); err != nil {
		return nil, err
	}
	return (knowledgeapp.AccessDescribeExecutor{
		Repositories: cx.WS.Reader.Lookup(cat.Require),
	}).Execute(cx.Context, knowledgeapp.AccessDescribeRequest{Pin: pin})
}

// ResourceLookup returns the state lookup injected by the composing transport.
// Without an injected lookup the seam falls back to the origin-bound HTTP
// adapter: the call target is the Schema origin pinned on each Binding, never
// a seam-invented address. The adapter itself reports TEMPORARY_UNAVAILABLE
// when a request carries no origin. (B6 曾把回退误改为硬失败，复核恢复原合同。)
func ResourceLookup(cx *Invocation) (knowledgeserving.StateLookup, error) {
	if cx.State != nil {
		return cx.State, nil
	}
	return knowledgeserving.NewHTTPStateLookup(nil), nil
}
