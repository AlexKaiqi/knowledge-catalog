package cli

import (
	"kc/kernel"
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
	if requiresState {
		if req.Continuation != "" {
			if _, _, ok := cx.WS.Index.StateView(repo.ID(), commitID); !ok {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "dynamic SearchView is no longer available; restart the search")
			}
		} else {
			request, err := stateRequestContextFrom(cx)
			if err != nil {
				return nil, err
			}
			if _, err := cx.WS.Index.RefreshState(cx.Context, repo, commitID, cx.State, request); err != nil {
				return nil, err
			}
		}
		return cx.WS.Index.SearchStateAt(repo, commitID, req)
	}
	if _, err := cx.WS.Index.Ensure(repo, commitID); err != nil {
		return nil, err
	}
	return cx.WS.Index.Search(repo, req)
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
	// Describing a projection is provider-independent. The configured service
	// provider may need to build the requested basis before it can describe it.
	if _, err := cx.WS.Index.Ensure(repo, commitID); err != nil {
		return nil, err
	}
	return cx.WS.Index.Describe(repo)
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
