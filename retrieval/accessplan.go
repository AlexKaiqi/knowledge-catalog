package retrieval

import (
	"kc/kernel"
	"kc/knowledge/reader"
)

// AccessPlan is Workspace-scoped introspection of logical access contracts.
// It is not a physical index definition and not a per-request RetrievalPlan.
type AccessPlan struct {
	WorkspaceID        string       `json:"workspaceId"`
	DefinitionRevision int          `json:"definitionRevision"`
	Specs              []AccessSpec `json:"specs"`
}

func PlanAccess(lookup reader.MemberLookup, pin reader.WorkspacePin) (AccessPlan, error) {
	plan := AccessPlan{
		WorkspaceID:        pin.WorkspaceID,
		DefinitionRevision: pin.Revision,
		Specs:              []AccessSpec{},
	}
	ids := make([]kernel.RepositoryID, 0, len(pin.Repositories))
	for id := range pin.Repositories {
		ids = append(ids, id)
	}
	sortRepoIDs(ids)
	for _, repositoryID := range ids {
		commit := pin.Repositories[repositoryID]
		repo, err := lookup(repositoryID)
		if err != nil {
			return AccessPlan{}, err
		}
		if !repo.HasCommit(commit) {
			return AccessPlan{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist in %s", commit, repositoryID)
		}
		report, err := reader.DescribeRepoSchema(repo, commit, "")
		if err != nil {
			return AccessPlan{}, err
		}
		plan.Specs = append(plan.Specs, AccessSpecFromReport(report))
	}
	return plan, nil
}

func sortRepoIDs(ids []kernel.RepositoryID) {
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
}
