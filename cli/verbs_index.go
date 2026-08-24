package cli

import (
	"kc/kernel"
	"kc/reader"
)

// Retrieval derivation verbs (layer ③). An index only locates; the caller reads
// the canonical unit back after a hit. One index belongs to one
// (repository, basisCommit) plus that repo's schema, never to a Workspace.

func indexVerbs() map[string]command {
	return map[string]command{
		"search":         {stage: stageGoverned, run: verbSearch},
		"describe-index": {stage: stageGoverned, run: verbDescribeIndex},
		"index-sync":     {stage: stageGoverned, run: verbIndexSync},
		"index-plan":     {stage: stageGoverned, run: verbIndexPlan},
	}
}

func verbSearch(cx *invocation) (any, error) {
	if servingWorkspace(cx.Flags) {
		return searchWorkspace(cx.WS, cx.Home, cx.Flags)
	}
	repositoryID, commitID, err := pinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	repo, err := cx.WS.Store.Knowledge(repositoryID, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return nil, err
	}
	if _, err := cx.WS.Index.Ensure(repo, commitID); err != nil {
		return nil, err
	}
	req, err := searchRequestFromFlags(cx.Flags)
	if err != nil {
		return nil, err
	}
	return cx.WS.Index.Search(repo, req)
}

func verbDescribeIndex(cx *invocation) (any, error) {
	repoID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	repo, err := cx.WS.Store.Knowledge(kernel.RepositoryID(repoID), kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return nil, err
	}
	return cx.WS.Index.Describe(repo)
}

func verbIndexSync(cx *invocation) (any, error) {
	repositoryID, commitID, err := pinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	repo, err := cx.WS.Store.Knowledge(repositoryID, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return nil, err
	}
	return cx.WS.Index.Ensure(repo, commitID)
}

// verbIndexPlan is one recipe per member repository at this Workspace resolution.
func verbIndexPlan(cx *invocation) (any, error) {
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
	return reader.PlanIndex(cat.RequireKnowledge, workspacePin(resolved))
}
