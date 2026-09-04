package cli

import (
	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

// Retrieval derivation verbs (layer ③). An index only locates; the caller reads
// the canonical unit back after a hit. One index belongs to one
// (repository, basisCommit) plus that Repository's schema, never to a Workspace.

func indexVerbs() map[string]command {
	return map[string]command{
		"search":          {stage: stageGoverned, run: verbSearch},
		"describe-index":  {stage: stageGoverned, run: verbDescribeIndex},
		"index-sync":      {stage: stageGoverned, run: verbIndexSync},
		"index-notify":    {stage: stageGoverned, run: verbIndexNotify},
		"describe-access": {stage: stageGoverned, run: verbDescribeAccess},
	}
}

func verbSearch(cx *invocation) (any, error) {
	if servingWorkspace(cx.Flags) {
		return searchWorkspace(cx)
	}
	repositoryID, commitID, err := pinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	repo, err := cx.WS.Reader.Require(repositoryID, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return nil, err
	}
	req, err := searchRequestFromFlags(cx.Flags)
	if err != nil {
		return nil, err
	}
	requiresState, err := cx.WS.Index.RequiresState(repo, commitID, req)
	if err != nil {
		return nil, err
	}
	var out retrieval.SearchResult
	if requiresState {
		if _, ok := cx.WS.Index.StateView(repo.ID(), commitID); !ok {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"State projection is not prepared")
		}
		out, err = cx.WS.Index.SearchStateAt(repo, commitID, req)
	} else {
		out, err = cx.WS.Index.SearchAt(repo, commitID, req)
	}
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
	repo, err := cx.WS.Reader.Require(repoID, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return nil, err
	}
	return cx.WS.Index.DescribeAt(repo, commitID)
}

func verbIndexSync(cx *invocation) (any, error) {
	repositoryID, commitID, err := pinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	repo, err := cx.WS.Reader.Require(repositoryID, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return nil, err
	}
	defer observeProjectionExecution(cx)()
	// Publish the immutable basis before advancing the live projection. A task
	// pinned to this commit must remain searchable after the source ref moves.
	if _, err := cx.WS.Index.EnsureAt(repo, commitID); err != nil {
		return nil, err
	}
	if cx.WS.Projection != nil {
		if err := cx.WS.Projection.Desire(repositoryID, commitID); err != nil {
			return nil, err
		}
		if err := cx.WS.Projection.CatchUp(cx.Context); err != nil {
			return nil, err
		}
	}
	snapshotSync, err := cx.WS.Index.Ensure(repo, commitID)
	if err != nil {
		return nil, err
	}
	if cx.State == nil {
		return snapshotSync, nil
	}
	request, err := stateRequestContextFrom(cx)
	if err != nil {
		return nil, err
	}
	stateSync, err := cx.WS.Index.RefreshState(cx.Context, repo, commitID, cx.State, request)
	if err != nil {
		return nil, err
	}
	return map[string]any{"snapshot": snapshotSync, "state": stateSync}, nil
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
	cx.WS.Projection.SetStateLookup(lookup)
	request, err := stateRequestContextFrom(cx)
	if err != nil {
		return nil, err
	}
	cx.WS.Projection.SetRequestContext(request)
	if err := cx.WS.Projection.Notify(notice); err != nil {
		return nil, err
	}
	if err := cx.WS.Projection.CatchUp(cx.Context); err != nil {
		return nil, err
	}
	repo, err := cx.WS.Reader.Require(notice.Repository, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return nil, err
	}
	head, err := repo.Head(notice.Ref)
	if err != nil {
		return nil, err
	}
	revision, ok := cx.WS.Index.StateView(notice.Repository, head)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State projection is not prepared")
	}
	return map[string]any{
		"repository": notice.Repository, "basisCommit": head, "revision": revision,
	}, nil
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

// verbDescribeAccess reports one logical AccessSpec per pinned member.
func verbDescribeAccess(cx *invocation) (any, error) {
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	workspaceID, err := cx.workspaceID()
	if err != nil {
		return nil, err
	}
	resolved, err := resolveOrReplay(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	pin := workspacePin(resolved)
	if err := requireCompleteWorkspaceRead(cx.Home, cx.Flags, pin, ""); err != nil {
		return nil, err
	}
	return retrieval.PlanAccess(cx.WS.Reader.Lookup(cat.Require), pin)
}
