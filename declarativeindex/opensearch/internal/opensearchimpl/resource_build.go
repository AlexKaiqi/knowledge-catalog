package opensearchimpl

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"time"

	"kc/kernel"
)

const (
	resourceRecordVersion = 5
	resourceBuilding      = "BUILDING"
	resourcePublishing    = "PUBLISHING"
	resourceAdopted       = "ADOPTED"
	resourceDeleting      = "DELETING"
	resourceDeleted       = "DELETED"
	buildLease            = 15 * time.Minute
)

// One resource record is the durable admission authority for this unique
// generation. Pending outlives a lost request response: deletion cannot infer
// that the admitted request stopped executing from an expired lease alone.
type generationResource struct {
	SupersededBy string          `json:"supersededBy,omitempty"`
	QueryLeases  int             `json:"queryLeases,omitempty"`
	Version      int             `json:"version"`
	Phase        string          `json:"phase"`
	LeaseUntil   int64           `json:"leaseUntil"`
	Pending      bool            `json:"pending"`
	Claims       []resourceClaim `json:"claims,omitempty"`
}

func validGeneration(generation string) bool {
	if len(generation) != 32 {
		return false
	}
	decoded, err := hex.DecodeString(generation)
	return err == nil && hex.EncodeToString(decoded) == generation
}

func (s *rebuildSession) resourceCtx() context.Context {
	if s.resourceContext != nil {
		return s.resourceContext
	}
	return context.Background()
}

func (s *rebuildSession) resourceID() string {
	return s.engine.controlID + "-resource-" + s.generation
}

func (s *rebuildSession) registerResource() error {
	record := controlFromMeta(s.engine.repository, s.physicalIndex, s.generation, "RESOURCE", s.meta, 0)
	record.Resource = &generationResource{Version: resourceRecordVersion, QueryLeases: 1, Phase: resourceBuilding, LeaseUntil: time.Now().Add(buildLease).UnixMilli()}
	_, err := s.engine.putControlRecord(s.resourceID(), record, nil)
	return err
}

func (s *rebuildSession) changeResource(change func(*generationResource) error) error {
	return s.changeResourceRecord(func(record *controlDoc) error { return change(record.Resource) })
}

func (s *rebuildSession) changeResourceRecord(change func(*controlDoc) error) error {
	for attempt := 0; attempt < 8; attempt++ {
		record, version, err := s.engine.loadControlRecord(s.resourceCtx(), s.resourceID())
		if err != nil {
			return err
		}
		if version == nil || !validGeneration(s.generation) || record.Repository != string(s.engine.repository) || record.ActiveIndex != s.physicalIndex || record.Generation != s.generation || record.State != "RESOURCE" || record.Resource == nil || !supportedResourceVersion(record.Resource.Version) || record.Resource.LeaseUntil <= 0 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource admission authority is unavailable or has different ownership: %s", s.physicalIndex)
		}
		if err := validateResourceProtection(record.Resource); err != nil {
			return err
		}
		if record.Resource.SupersededBy == s.generation {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource deletion proof names itself")
		}
		record.Resource.Version = resourceRecordVersion
		if err := change(&record); err != nil {
			return err
		}
		_, err = s.engine.putControlRecordContext(s.resourceCtx(), s.resourceID(), record, version)
		if kernel.CodeOf(err) != kernel.ErrNonFastForward {
			return err
		}
	}
	return kernel.Fail(kernel.ErrTemporaryUnavailable, "resource admission changed repeatedly: %s", s.physicalIndex)
}

func activeBuild(resource *generationResource) error {
	if resource.Phase != resourceBuilding || resource.Pending || resource.LeaseUntil <= time.Now().UnixMilli() {
		return kernel.Fail(kernel.ErrPreconditionFailed, "resource build lease expired, closed, or has an unresolved physical request")
	}
	return nil
}

func (s *rebuildSession) resourceIO(operation func() error) error {
	if err := s.changeResource(func(resource *generationResource) error {
		if err := activeBuild(resource); err != nil {
			return err
		}
		resource.Pending = true
		resource.LeaseUntil = time.Now().Add(buildLease).UnixMilli()
		return nil
	}); err != nil {
		return err
	}
	operationErr := operation()
	var completed *completedPhysicalError
	if operationErr != nil && !errors.As(operationErr, &completed) {
		return operationErr // Unknown completion keeps the durable admission outstanding.
	}
	completionErr := s.changeResource(func(resource *generationResource) error {
		if resource.Phase != resourceBuilding || !resource.Pending {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource admission changed while physical request was running")
		}
		resource.Pending = false
		return nil
	})
	return errors.Join(completionErr, operationErr)
}

func (s *rebuildSession) resourcePhase(phase string) error {
	return s.changeResourceRecord(func(record *controlDoc) error {
		resource := record.Resource
		switch phase {
		case resourcePublishing:
			if err := activeBuild(resource); err != nil {
				return err
			}
			record.ObjectCount = s.expected
			if s.preparation != nil {
				resource.Claims = append(resource.Claims, resourceClaim{ID: s.preparation.id, Phase: claimPrepared, Until: time.Now().Add(buildLease).UnixMilli()})
			}
		case resourceAdopted:
			if resource.Phase == resourceAdopted {
				return nil
			}
			if resource.Phase != resourcePublishing || resource.Pending {
				return kernel.Fail(kernel.ErrPreconditionFailed, "resource publication admission changed")
			}
		default:
			return kernel.Fail(kernel.ErrPreconditionFailed, "unsupported resource phase transition")
		}
		resource.Phase = phase
		return nil
	})
}

// Only an unpublished, drained candidate can enter deletion. Publishing
// candidates require an authoritative rejection; expiration cannot grant it.
func (s *rebuildSession) discardResource() error {
	return s.discardResourceAt(false)
}

func (s *rebuildSession) discardResourceAt(expiredOnly bool) error {
	historical, complete := false, false
	if err := s.changeResource(func(resource *generationResource) error {
		historical = resource.SupersededBy != ""
		complete = resource.Phase == resourceDeleted
		if complete {
			if resource.Pending || len(resource.Claims) > 0 {
				return kernel.Fail(kernel.ErrPreconditionFailed, "completed deletion has contradictory protection")
			}
			return nil
		}
		if resource.Pending || len(resource.Claims) > 0 || (resource.Phase != resourceBuilding && resource.Phase != resourceDeleting && !(resource.Phase == resourcePublishing && s.publicationRejected)) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource still has a pending request or publication protection")
		}
		if expiredOnly && resource.Phase == resourceBuilding && resource.LeaseUntil > time.Now().UnixMilli() {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource build lease is still active")
		}
		resource.Phase = resourceDeleting
		return nil
	}); err != nil {
		return err
	}
	if complete {
		return nil
	}
	if historical {
		if _, err := s.supersedingRoot(); err != nil {
			if errors.Is(err, errResourceProtected) {
				return kernel.Fail(kernel.ErrPreconditionFailed, "historical deletion target was unexpectedly restored as ROOT")
			}
			return err
		}
	}
	status, body, err := s.engine.doContext(s.resourceCtx(), http.MethodDelete, "/"+url.PathEscape(s.physicalIndex), nil)
	if err != nil {
		return err
	}
	if status >= 300 && status != http.StatusNotFound {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "resource deletion returned HTTP %d: %s", status, body)
	}
	return s.changeResource(func(resource *generationResource) error {
		if resource.Phase == resourceDeleted {
			return nil
		}
		if resource.Phase != resourceDeleting || resource.Pending {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource deletion authority changed")
		}
		resource.Phase = resourceDeleted
		return nil
	})
}

func (e *openSearchEngine) sendBulkGuarded(payload []byte, refresh bool, guard func(func() error) error) (int, error) {
	if guard == nil {
		return e.sendBulk(payload, refresh)
	}
	var delta int
	err := guard(func() error {
		var err error
		delta, err = e.sendBulk(payload, refresh)
		return err
	})
	return delta, err
}
