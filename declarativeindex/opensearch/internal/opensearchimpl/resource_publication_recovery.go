package opensearchimpl

import (
	"reflect"
	"time"

	ports "kc/discovery/projection"
	"kc/kernel"
)

// Only a complete authoritative READY record proves publication. A missing or
// different ROOT cannot prove rejection: the original request may still arrive.
func (s *rebuildSession) recoverResourcePublication() error {
	root, version, err := s.engine.loadControlContext(s.resourceCtx())
	if err != nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "resource publication ROOT is unknown: %v", err)
	}
	if version == nil || root.State != ports.ProjectionStateReady || root.Repository != string(s.engine.repository) || root.Generation != s.generation || root.ActiveIndex != s.physicalIndex {
		return s.rejectExpiredPublication(root, version)
	}
	err = s.changeResourceRecord(func(record *controlDoc) error {
		resource := record.Resource
		if resource.Phase == resourceAdopted {
			return nil
		}
		if resource.Phase != resourcePublishing || resource.Pending || root.ObjectCount != record.ObjectCount || !reflect.DeepEqual(metaFromControl(root), readyResourceMeta(*record)) || (resource.QueryLeases == 1 && root.QueryLeaseVersion != 1) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource publication READY evidence is incomplete")
		}
		resource.Phase = resourceAdopted
		return nil
	})
	if err != nil {
		return err
	}
	return errResourceProtected
}

// Rejection requires a fresh authoritative write fence, not merely a different
// pointer or an expired lease. Old ROOT CAS requests cannot cross this fence.
func (s *rebuildSession) rejectExpiredPublication(root controlDoc, version *controlVersion) error {
	if s.engine.namespace == "" || version == nil || root.Repository != string(s.engine.repository) || !validGeneration(root.Generation) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "publication rejection requires an owned existing ROOT")
	}
	switch root.State {
	case ports.ProjectionStateReady, ports.ProjectionStateBuilding, ports.ProjectionStateFailed, ports.ProjectionStateUpdating:
	default:
		return kernel.Fail(kernel.ErrPreconditionFailed, "publication rejection ROOT state is unknown")
	}
	if (root.ActiveIndex != "" && root.ActiveIndex != s.engine.prefix+"-g-"+root.Generation) || (root.State == ports.ProjectionStateReady && root.ActiveIndex == "") {
		return kernel.Fail(kernel.ErrPreconditionFailed, "publication rejection ROOT physical identity is unknown")
	}
	if err := s.changeResource(func(resource *generationResource) error {
		prunePreparedClaims(resource)
		if resource.Phase != resourcePublishing || resource.Pending || len(resource.Claims) > 0 || resource.QueryLeases != 1 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "publication has unresolved protection or legacy readers")
		}
		if resource.LeaseUntil > time.Now().UnixMilli() {
			return errResourceProtected
		}
		return nil
	}); err != nil {
		return err
	}
	if _, err := s.engine.putControlRecordContext(s.resourceCtx(), s.engine.controlID, root, version); err != nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "publication rejection fence is unconfirmed: %v", err)
	}
	s.publicationRejected = true
	return s.discardResourceAt(true)
}
