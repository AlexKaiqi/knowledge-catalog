package opensearchimpl

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"reflect"
	"slices"
	"time"

	ports "kc/discovery/projection"
	"kc/kernel"
)

const (
	claimRootQuery        = "ROOT_QUERY"
	claimRootContinuation = "ROOT_CONTINUATION"
	claimPrepared         = "PREPARED"
	claimAdmitted         = "ADMITTED"
	claimAccepted         = "ACCEPTED"
	claimQuery            = "QUERY"
	claimContinuation     = "CONTINUATION"
)

type resourceClaim struct {
	ID       string `json:"id"`
	Phase    string `json:"phase"`
	Until    int64  `json:"until"`
	Evidence []byte `json:"evidence,omitempty"`
}

type preparedResource struct {
	observation *observationAdmission
	rootQuery   *controlDoc
	queryAnchor *resourceClaim
	session     *rebuildSession
	id          string
	ready       controlDoc
}

func validateResourceClaims(claims []resourceClaim) error {
	seen := map[string]bool{}
	for _, claim := range claims {
		if !validGeneration(claim.ID) || seen[claim.ID] || claim.Until <= 0 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource publication claim has invalid identity or lease")
		}
		seen[claim.ID] = true
		switch claim.Phase {
		case claimPrepared:
			if len(claim.Evidence) != 0 {
				return kernel.Fail(kernel.ErrPreconditionFailed, "prepared resource claim unexpectedly has publication evidence")
			}
		case claimAdmitted, claimAccepted, claimQuery, claimContinuation, claimRootQuery, claimRootContinuation:
			if len(claim.Evidence) == 0 {
				return kernel.Fail(kernel.ErrPreconditionFailed, "admitted resource claim has no authority evidence")
			}
		default:
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource publication claim has unknown protection")
		}
	}
	return nil
}

func prunePreparedClaims(resource *generationResource) {
	now := time.Now().UnixMilli()
	resource.Claims = slices.DeleteFunc(resource.Claims, func(claim resourceClaim) bool {
		return claim.Phase == claimPrepared && claim.Until > 0 && claim.Until <= now
	})
}

func (e *openSearchEngine) PrepareResource(ctx context.Context, expected ports.Meta) (ports.PreparedResource, error) {
	if err := e.requireMutableProjection(); err != nil {
		return nil, err
	}
	if !validGeneration(expected.Generation) || expected.State != ports.ProjectionStateReady {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "resource preparation requires an exact ready generation")
	}
	actual, err := e.LoadMetaContext(ctx)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(actual, expected) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "prepared projection metadata changed")
	}
	ready, _, err := e.loadControlContext(ctx)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(metaFromControl(ready), expected) || ready.Repository != string(e.repository) || ready.ActiveIndex != e.prefix+"-g-"+expected.Generation {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "prepared resource lost exact ready metadata")
	}
	id, err := newResourceID()
	if err != nil {
		return nil, err
	}
	ticket := &preparedResource{id: id, ready: ready, session: &rebuildSession{
		engine: e, physicalIndex: e.prefix + "-g-" + expected.Generation, generation: expected.Generation, resourceContext: ctx,
	}}
	if err := ticket.session.changeResourceRecord(func(record *controlDoc) error {
		resource := record.Resource
		if resource.Phase != resourceAdopted || resource.Pending {
			return kernel.Fail(kernel.ErrPreconditionFailed, "prepared generation is not immutably available")
		}
		prunePreparedClaims(resource)
		preserved := *record
		preserved.State = ports.ProjectionStateReady
		if len(resource.Claims) > 0 && !reflect.DeepEqual(metaFromControl(preserved), metaFromControl(ready)) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource metadata differs from an existing frozen reference")
		}
		if len(resource.Claims) == 0 {
			*record = ready
			record.State = "RESOURCE"
			record.Resource = resource
		}

		resource.Claims = append(resource.Claims, resourceClaim{ID: ticket.id, Phase: claimPrepared, Until: time.Now().Add(buildLease).UnixMilli()})
		return nil
	}); err != nil {
		return nil, err
	}
	// An incremental writer could have completed between the first root read
	// and acquisition. Claims now exclude further writes; verify the actual
	// root again before returning a usable fixed-generation ticket.
	actual, err = e.LoadMetaContext(ctx)
	if err != nil || !reflect.DeepEqual(actual, expected) {
		// Failure may leave a bounded PREPARED claim; never release an unknown
		// admission. No admission is possible until the ticket is returned.
		if err != nil {
			return nil, err
		}
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "projection changed while acquiring its resource")
	}
	return ticket, nil
}

func (p *preparedResource) change(ctx context.Context, update func(*resourceClaim) error, cancel bool) error {
	session := *p.session
	session.resourceContext = ctx
	return session.changeResource(func(resource *generationResource) error {
		if resource.Phase != resourceAdopted || resource.Pending {
			return kernel.Fail(kernel.ErrPreconditionFailed, "prepared resource is no longer available")
		}
		for i := range resource.Claims {
			if resource.Claims[i].ID != p.id {
				continue
			}
			if err := update(&resource.Claims[i]); err != nil {
				return err
			}
			if cancel {
				resource.Claims = slices.Delete(resource.Claims, i, i+1)
			}
			return nil
		}
		if cancel {
			return nil
		}
		return kernel.Fail(kernel.ErrPreconditionFailed, "prepared resource ticket expired or was released")
	})
}

func (p *preparedResource) Admit(ctx context.Context, evidence []byte) error {
	if len(evidence) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "publication admission requires authority evidence")
	}
	if err := p.admitClaim(ctx, evidence); err != nil {
		return err
	}
	return p.publishBinding(ctx, evidence)
}

func (p *preparedResource) admitClaim(ctx context.Context, evidence []byte) error {
	return p.change(ctx, func(claim *resourceClaim) error {
		if claim.Phase == claimAdmitted || claim.Phase == claimAccepted {
			if bytes.Equal(claim.Evidence, evidence) {
				return nil
			}
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource ticket belongs to different publication evidence")
		}
		if claim.Phase != claimPrepared || claim.Until <= time.Now().UnixMilli() {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource preparation lease expired")
		}
		claim.Phase, claim.Evidence = claimAdmitted, bytes.Clone(evidence)
		claim.Until = time.Now().Add(buildLease).UnixMilli()
		return nil
	}, false)
}

func (p *preparedResource) Confirm(ctx context.Context) error {
	return p.change(ctx, func(claim *resourceClaim) error {
		if claim.Phase != claimAdmitted && claim.Phase != claimAccepted {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource publication was never admitted")
		}
		claim.Phase = claimAccepted
		return nil
	}, false)
}

func (p *preparedResource) Cancel(ctx context.Context) error {
	return p.change(ctx, func(claim *resourceClaim) error {
		if claim.Phase != claimPrepared {
			return kernel.Fail(kernel.ErrPreconditionFailed, "admitted publication requires authority recovery")
		}
		return nil
	}, true)
}

func (p *preparedResource) Close() error { return nil }

func newResourceID() (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(nonce[:]), nil
}

func supportedResourceVersion(version int) bool {
	return version == 1 || version == 2 || version == 3 || version == 4 || version == resourceRecordVersion
}

func validateResourceProtection(resource *generationResource) error {
	if resource == nil || !supportedResourceVersion(resource.Version) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "resource protection version is unknown")
	}
	if resource.QueryLeases < 0 || resource.QueryLeases > 1 || (resource.QueryLeases != 0 && resource.Version < 5) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "resource query protection protocol is unknown")
	}
	if resource.SupersededBy != "" && (resource.Version < 5 || resource.QueryLeases != 1 || !validGeneration(resource.SupersededBy) || (resource.Phase != resourceDeleting && resource.Phase != resourceDeleted)) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "resource historical deletion proof is invalid")
	}
	if resource.Version == 1 && len(resource.Claims) != 0 {
		return kernel.Fail(kernel.ErrPreconditionFailed, "legacy resource record cannot own claims")
	}
	for _, claim := range resource.Claims {
		if (claim.Phase == claimRootQuery || claim.Phase == claimRootContinuation) && (resource.Version < 5 || resource.QueryLeases != 1) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "ROOT query leases require proven protocol version 5 coverage")
		}
		if claim.Phase == claimContinuation && resource.Version < 4 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "continuation protection requires resource protocol version 4")
		}
		if claim.Phase == claimQuery && resource.Version < 3 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "query protection requires resource protocol version 3")
		}
	}
	return validateResourceClaims(resource.Claims)
}
