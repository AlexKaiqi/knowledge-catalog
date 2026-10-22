package lakefsimpl

import (
	"kc/kernel"
	"net/http"
)

// Only an actual published candidate can acknowledge a failed lock acquisition.
// A no-change verifier must acquire its own lock even when candidate==expected.
func (r *Repository) acquirePublicationLock(name string, candidate, expected kernel.CommitID, allowPublished bool) (bool, error) {
	lock := r.publicationLock(name)
	if err := r.createBranch(lock, candidate, true); err != nil {
		if statusOf(err) != http.StatusConflict {
			return false, err
		}
		current, found, readErr := r.getBranch(name)
		if readErr != nil {
			return false, readErr
		}
		if !found {
			return false, kernel.Fail(kernel.ErrVersionUnresolved, "ref refs/heads/%s does not exist", name)
		}
		lockHead, _, lockErr := r.getBranch(lock)
		if lockErr != nil {
			return false, lockErr
		}
		if allowPublished && current == candidate {
			return false, nil
		}
		if current != expected {
			return false, kernel.Fail(kernel.ErrNonFastForward,
				"ref refs/heads/%s moved: expected commit %s, actual %s", name, expected, current)
		}
		return false, kernel.Fail(kernel.ErrTemporaryUnavailable,
			"lakefs publication lock for refs/heads/%s is held (target %s, candidate %s); stop all publishers before RecoverLakeFSPublication", name, current, lockHead)
	}
	return true, nil
}
