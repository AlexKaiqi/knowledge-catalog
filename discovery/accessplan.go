package discovery

import (
	"context"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// AccessPlan is Workspace-scoped introspection of logical access contracts.
// It is not a physical index definition and not a per-request RetrievalPlan.
type AccessPlan struct {
	SetID              string               `json:"setId"`
	DefinitionRevision int                  `json:"definitionRevision"`
	Items              []reader.DatasetItem `json:"items,omitempty"`
	Specs              []AccessSpec         `json:"specs"`
}

type AccessContractProvider func(objectmodel.Repository, kernel.CommitID) (AccessSpec, error)

func PlanAccess(lookup reader.MemberLookup, pin reader.DatasetPin, providers ...AccessContractProvider) (AccessPlan, error) {
	return PlanAccessContext(context.Background(), lookup, pin, providers...)
}

// PlanAccessContext compiles one AccessSpec per pinned member, stopping when
// the caller's context is done. Compiling a member contract is bounded per
// member but the number of members is caller data, so the plan must stay
// interruptible: a transport timeout must not leave the request running.
func PlanAccessContext(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, providers ...AccessContractProvider) (AccessPlan, error) {
	if err := ctx.Err(); err != nil {
		return AccessPlan{}, err
	}
	if len(pin.Items) == 0 {
		return AccessPlan{}, kernel.Fail(kernel.ErrDatasetInvalid, "dataset pin has no published file list")
	}
	plan := AccessPlan{
		SetID:              pin.SetID,
		DefinitionRevision: pin.Revision,
		Items:              append([]reader.DatasetItem(nil), pin.Items...),
		Specs:              []AccessSpec{},
	}
	ids := make([]kernel.RepositoryID, 0, len(pin.Repositories))
	for id := range pin.Repositories {
		ids = append(ids, id)
	}
	sortRepoIDs(ids)
	scope := reader.Open(lookup, pin)
	for _, repositoryID := range ids {
		if err := ctx.Err(); err != nil {
			return AccessPlan{}, err
		}
		commit := pin.Repositories[repositoryID]
		repo, err := scope.Member(repositoryID)
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
		spec := AccessSpecFromReport(report)
		if len(providers) > 0 && providers[0] != nil {
			spec, err = providers[0](repo, commit)
			if err != nil {
				return AccessPlan{}, err
			}
		}
		plan.Specs = append(plan.Specs, spec)
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
