package lakefsimpl

import (
	"net/http"
	"net/url"
	"strings"

	"kc/kernel"
)

// RecoverPublication only releases a matching coordination branch. The
// operator must stop every publisher first: stock lakeFS has no fencing token
// to prevent an already running lock holder from continuing after deletion.
func (r *Repository) RecoverPublication(targetRef string, expectedTarget, expectedCandidate kernel.CommitID) error {
	name, ok := r.branchName(targetRef)
	if !ok || !strings.HasPrefix(targetRef, "refs/heads/") || name == "" || expectedTarget == "" || expectedCandidate == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "publication recovery requires a full branch ref and observed target and candidate commits")
	}
	actual, found, err := r.getBranch(name)
	if err != nil {
		return err
	}
	if !found {
		return kernel.Fail(kernel.ErrVersionUnresolved, "publication target %s is missing", targetRef)
	}
	if actual != expectedTarget {
		return kernel.Fail(kernel.ErrNonFastForward, "publication target moved: expected %s, actual %s", expectedTarget, actual)
	}
	lock := r.publicationLock(name)
	candidate, found, err := r.getBranch(lock)
	if err != nil || !found {
		return err
	}
	if candidate != expectedCandidate {
		return kernel.Fail(kernel.ErrPreconditionFailed, "publication lock changed: expected %s, actual %s", expectedCandidate, candidate)
	}
	_, _, err = r.client.doJSON(http.MethodDelete, r.client.repositoryPath("branches/"+url.PathEscape(lock))+"?force=true", nil, nil)
	return err
}
