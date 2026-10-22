package opensearchimpl

import (
	"errors"
	"slices"
	"time"

	ports "kc/discovery/projection"
	"kc/kernel"
)

var errResourceProtected = errors.New("resource has an effective consumer protection")

// READY replacement plus the resource CAS excludes both old ROOT writers and
// all managed query/publication admissions. Missing ROOT is not replacement.
func (s *rebuildSession) supersedingRoot() (controlDoc, error) {
	if s.engine.namespace == "" {
		return controlDoc{}, kernel.Fail(kernel.ErrPreconditionFailed, "historical reclamation requires an exclusively owned namespace")
	}
	root, version, err := s.engine.loadControlContext(s.resourceCtx())
	if err != nil {
		return controlDoc{}, kernel.Fail(kernel.ErrPreconditionFailed, "historical resource ROOT is unknown: %v", err)
	}
	if version == nil || root.State != ports.ProjectionStateReady || root.Repository != string(s.engine.repository) || !validGeneration(root.Generation) || root.ActiveIndex != s.engine.prefix+"-g-"+root.Generation {
		return controlDoc{}, kernel.Fail(kernel.ErrPreconditionFailed, "historical resource has no proven READY ROOT")
	}
	if root.Generation == s.generation {
		return controlDoc{}, errResourceProtected
	}
	if root.QueryLeaseVersion != 1 {
		return controlDoc{}, kernel.Fail(kernel.ErrPreconditionFailed, "superseding ROOT has unknown query coverage")
	}
	return root, nil
}

func (s *rebuildSession) discardHistoricalResource() error {
	if s.engine.namespace == "" {
		return kernel.Fail(kernel.ErrPreconditionFailed, "historical reclamation requires an exclusively owned namespace")
	}
	latest, _, err := s.engine.loadControlRecord(s.resourceCtx(), s.resourceID())
	if err != nil {
		return err
	}
	if latest.Resource != nil && latest.Resource.SupersededBy != "" && (latest.Resource.Phase == resourceDeleting || latest.Resource.Phase == resourceDeleted) {
		return s.discardResourceAt(true)
	}
	root, err := s.supersedingRoot()
	if err != nil {
		return err
	}
	if err := s.changeResource(func(resource *generationResource) error {
		if resource.Version < 5 || resource.QueryLeases != 1 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "historical resource has untracked legacy readers")
		}
		if resource.Phase == resourceDeleting || resource.Phase == resourceDeleted {
			if resource.SupersededBy == "" || resource.Pending || len(resource.Claims) > 0 {
				return kernel.Fail(kernel.ErrPreconditionFailed, "historical deletion has contradictory protection")
			}
			return nil
		}
		if resource.Phase != resourceAdopted || resource.Pending {
			return kernel.Fail(kernel.ErrPreconditionFailed, "historical resource has unresolved writes or publication")
		}
		now := time.Now().UnixMilli()
		// Only bounded, never-admitted or query protections expire locally.
		// Publication admissions and accepted references require their authority.
		resource.Claims = slices.DeleteFunc(resource.Claims, func(claim resourceClaim) bool {
			return (claim.Phase == claimPrepared || claim.Phase == claimQuery || claim.Phase == claimContinuation || claim.Phase == claimRootQuery || claim.Phase == claimRootContinuation) && claim.Until <= now
		})
		if len(resource.Claims) > 0 {
			return errResourceProtected
		}
		resource.SupersededBy = root.Generation
		resource.Phase = resourceDeleting
		return nil
	}); err != nil {
		return err
	}
	return s.discardResourceAt(true)
}
