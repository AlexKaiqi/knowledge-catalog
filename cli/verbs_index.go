package cli

import (
	"kc/catalog"
	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	knowledgeserving "kc/knowledge/serving"
	"kc/knowledgeapp"
	"kc/retrieval"
)

// Retrieval derivation verbs (layer ③). An index only locates; the caller reads
// the canonical unit back after a hit. One index belongs to one
// (repository, basisCommit) plus that Repository's schema, never to a Workspace.

func indexVerbs() map[string]command {
	return map[string]command{
		"knowledge-search":                {stage: stageGoverned, run: verbSearch},
		"operations-projection-describe":  {stage: stageGoverned, run: verbDescribeIndex},
		"operations-projection-sync":      {stage: stageGoverned, run: verbIndexSync},
		"operations-projection-notice":    {stage: stageGoverned, run: verbIndexNotify},
		"operations-access-spec-describe": {stage: stageGoverned, run: verbDescribeAccess},
	}
}

func verbSearch(cx *invocation) (any, error) {
	// The HTTP server passes its request-time embedding provider through the
	// internal _embedder stamp (like _search-request). Model providers live
	// only in the server; the plain CLI leaves the stamp unset and semantic
	// recall fails closed there.
	embedder, _ := cx.Flags["_embedder"].(retrieval.Embedder)
	if servingWorkspace(cx.Flags) {
		return searchWorkspace(cx)
	}
	repositoryID, commitID, err := pinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	req, err := searchRequestFromFlags(cx.Flags)
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
	return deliverSearchResult(cx.Home, cx.Flags, out)
}

func verbDescribeIndex(cx *invocation) (any, error) {
	repoID, commitID, err := pinCommit(cx.WS, cx.Flags)
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

func verbIndexSync(cx *invocation) (any, error) {
	repositoryID, commitID, err := pinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	result, err := (knowledgeapp.ProjectionSyncExecutor{
		Repositories: cx.WS.Reader,
		Projection:   cx.WS.Index,
		Controller:   cx.WS.Projection,
		Observe: func() func() {
			return observeProjectionExecution(cx)
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

func verbIndexNotify(cx *invocation) (any, error) {
	notice, err := changeNoticeFromFlags(cx.Flags)
	if err != nil {
		return nil, err
	}
	if _, err := requireRepo(cx.WS, string(notice.Repository)); err != nil {
		return nil, err
	}
	if cx.WS.Projection == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection controller is not configured")
	}
	lookup, err := resourceLookup(cx)
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

func changeNoticeFromFlags(flags map[string]FlagValue) (index.ChangeNotice, error) {
	repo, err := RequireFlag(flags, "repo")
	if err != nil {
		return index.ChangeNotice{}, err
	}
	notice := index.ChangeNotice{
		Repository:     kernel.RepositoryID(repo),
		Ref:            FlagString(flags, "ref"),
		SourceRevision: FlagString(flags, "source-revision"),
	}
	if object := FlagString(flags, "object"); object != "" {
		kind := knowledge.AddressKind(FlagString(flags, "kind"))
		if kind == "" {
			if FlagString(flags, "aspect") != "" {
				kind = knowledge.KindAspect
			} else {
				kind = knowledge.KindEntity
			}
		}
		notice.Address = &knowledge.Address{
			Kind: kind, ObjectID: knowledge.ObjectID(object), AspectName: FlagString(flags, "aspect"),
		}
	}
	return notice, index.ValidateChangeNotice(notice)
}

// verbDescribeAccess reports one logical AccessSpec per pinned member, or the
// single AccessSpec of one Repository's published basis.
func verbDescribeAccess(cx *invocation) (any, error) {
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	var resolved catalog.ResolvedKnowledgeSet
	if repository := FlagString(cx.Flags, "repo"); repository != "" && FlagString(cx.Flags, "dataset") == "" {
		resolved, err = cat.ResolveDefinition(catalog.KnowledgeSet{
			Revision: 1,
			Sources:  []catalog.KnowledgeSetSource{{Repository: kernel.RepositoryID(repository), Selector: defaultRef}},
		})
	} else {
		setID, workspaceErr := cx.setID()
		if workspaceErr != nil {
			return nil, workspaceErr
		}
		resolved, err = resolveOrReplay(cx.WS, cx.Home, cat, setID, cx.Flags)
	}
	if err != nil {
		return nil, err
	}
	pin := knowledgeSetPin(resolved)
	if err := requireCompleteWorkspaceRead(cx.Home, cx.Flags, pin, ""); err != nil {
		return nil, err
	}
	return (knowledgeapp.AccessDescribeExecutor{
		Repositories: cx.WS.Reader.Lookup(cat.Require),
	}).Execute(cx.Context, knowledgeapp.AccessDescribeRequest{Pin: pin})
}
