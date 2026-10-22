package lakefsimpl

import (
	"kc/kernel"
	"kc/snapshot/internal/storecontract"
)

// VerifyTreeWrite checks backend write permissions through the same isolated
// staging primitive, then checks the original target under its publication
// lock. No commit or target ref mutation is issued, even on failure.
func (r *Repository) VerifyTreeWrite(cs storecontract.TreeChangeSet, bulk bool) (err error) {
	defer func() {
		if err != nil {
			err = storecontract.WriteNotApplied(err)
		}
	}()
	name, err := r.checkTreeWriteTarget(cs)
	if err != nil {
		return err
	}
	wip := r.nextWIP(cs)
	if err := r.createBranch(wip, cs.BaseCommit, true); err != nil {
		return err
	}
	defer r.deleteBranch(wip)
	write := r.client.stage
	if bulk {
		write = r.client.uploadObject
	}
	if err := r.applyChanges(wip, cs.Changes, write); err != nil {
		return err
	}
	acquired, err := r.acquirePublicationLock(name, cs.BaseCommit, cs.ExpectedTargetCommit, false)
	if err != nil {
		return err
	}
	if !acquired {
		return kernel.Fail(kernel.ErrPreconditionFailed, "no-change verifier did not acquire its publication lock")
	}
	defer r.deleteBranch(r.publicationLock(name))
	_, err = r.checkTreeWriteTarget(cs)
	return err
}
