package reader

import "kc/kernel"

// AccessPlan is Workspace-scoped introspection of logical access contracts.
// It is not a physical index definition and not a per-request RetrievalPlan.
type AccessPlan struct {
	WorkspaceID        string       `json:"workspaceId"`
	DefinitionRevision int          `json:"definitionRevision"`
	Specs              []AccessSpec `json:"specs"`
}

func PlanAccess(lookup MemberLookup, pin WorkspacePin) (AccessPlan, error) {
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
		report, err := DescribeRepoSchema(repo, commit, "")
		if err != nil {
			return AccessPlan{}, err
		}
		plan.Specs = append(plan.Specs, AccessSpecFromReport(report))
	}
	return plan, nil
}
