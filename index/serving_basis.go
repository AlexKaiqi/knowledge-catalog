package index

import (
	"context"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
)

// PrepareServingBasis prepares a retained immutable Snapshot projection before
// an application publishes a Dataset. It does not advance the live projection.
// Plain file datasets and deployments without retrieval have nothing to build.
func (c *Controller) PrepareServingBasis(ctx context.Context, repo knowledge.Repository, commit kernel.CommitID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.index == nil {
		return nil
	}
	report, err := reader.DescribeRepoSchema(repo, commit, "")
	if err != nil {
		return err
	}
	if len(report.Schemas) == 0 {
		// Built-in Relation knowledge does not require a custom schema. Only
		// an explicitly empty knowledge namespace proves there is no projection
		// to prepare; absence of schema/* alone is not that proof.
		if pager, ok := repo.(knowledge.SnapshotObjectPager); ok {
			page, err := pager.ObjectIDsPage(commit, 1, "")
			if err != nil {
				return err
			}
			if len(page.ObjectIDs) == 0 && page.Exhausted {
				return nil
			}
		}
	}
	if _, err := c.index.EnsureAt(repo, commit); err != nil {
		return err
	}
	// Live observations are not frozen Dataset content. Their authenticated
	// refresh/notice lane owns readiness; publication must not impersonate a
	// consumer or make external calls with an empty background identity.
	return ctx.Err()
}
