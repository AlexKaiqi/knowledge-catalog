package opensearchimpl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"

	ports "kc/discovery/projection"
	"kc/kernel"
)

const publicationBindingState = "PUBLICATION_BINDING"

func (e *openSearchEngine) requireMutableProjection() error {
	if len(e.fixedControl) > 0 {
		return kernel.Fail(kernel.ErrPreconditionFailed, "a fixed publication query view cannot mutate projection state")
	}
	return nil
}

func (e *openSearchEngine) publicationBindingID(evidence []byte) string {
	digest := sha256.Sum256(evidence)
	return e.controlID + "-publication-" + hex.EncodeToString(digest[:])
}

// Definite absence is distinct from unavailable transport or unknown ownership.
// Only this local proof allows maintenance to replace an accepted binding.
type missingPublicationResource struct{ cause error }

func (e *missingPublicationResource) Error() string { return e.cause.Error() }
func (e *missingPublicationResource) Unwrap() error { return e.cause }

// A ready binding is durable before admission returns to the Dataset CAS.
// Evidence remains opaque: the provider indexes no Dataset/Object protocol.
func (p *preparedResource) publishBinding(ctx context.Context, evidence []byte) error {
	e := p.session.engine
	id := e.publicationBindingID(evidence)
	existing, version, err := e.loadControlRecord(ctx, id)
	if err != nil {
		return err
	}
	if version != nil {
		return p.checkPublicationBinding(existing, evidence)
	}
	if p.ready.State != ports.ProjectionStateReady || p.ready.Generation != p.session.generation || p.ready.ActiveIndex != p.session.physicalIndex {
		return kernel.Fail(kernel.ErrPreconditionFailed, "prepared publication binding lacks exact ready metadata")
	}
	record := p.ready
	record.State = publicationBindingState
	record.PublicationVersion, record.PublicationTicket, record.PublicationEvidence = 1, p.id, bytes.Clone(evidence)
	if _, err := e.putControlRecordContext(ctx, id, record, nil); err != nil {
		if kernel.CodeOf(err) != kernel.ErrNonFastForward {
			return err
		}
		existing, _, readErr := e.loadControlRecord(ctx, id)
		if readErr != nil {
			return readErr
		}
		return p.checkPublicationBinding(existing, evidence)
	}
	return nil
}

func (p *preparedResource) checkPublicationBinding(record controlDoc, evidence []byte) error {
	if record.State != publicationBindingState || record.PublicationVersion != 1 || record.Repository != string(p.session.engine.repository) || record.Generation != p.session.generation || record.ActiveIndex != p.session.physicalIndex || record.PublicationTicket != p.id || !bytes.Equal(record.PublicationEvidence, evidence) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "publication binding has different immutable ownership or evidence")
	}
	if p.ready.State != "" {
		ready := record
		ready.State = ports.ProjectionStateReady
		if !reflect.DeepEqual(metaFromControl(ready), metaFromControl(p.ready)) || ready.ObjectCount != p.ready.ObjectCount {
			return kernel.Fail(kernel.ErrPreconditionFailed, "publication binding has different prepared metadata")
		}
	}
	return nil
}

func (e *openSearchEngine) BindPublication(ctx context.Context, evidence []byte) (ports.Engine, error) {
	if len(evidence) == 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "publication binding requires authority evidence")
	}
	record, version, err := e.loadControlRecord(ctx, e.publicationBindingID(evidence))
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, &missingPublicationResource{kernel.Fail(kernel.ErrCapabilityUnsatisfied, "accepted publication resource binding is unavailable")}
	}
	if !validGeneration(record.Generation) || !validGeneration(record.PublicationTicket) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "publication binding has invalid resource identity")
	}
	ticket := &preparedResource{id: record.PublicationTicket, session: &rebuildSession{engine: e, physicalIndex: e.prefix + "-g-" + record.Generation, generation: record.Generation}}
	if err := ticket.checkPublicationBinding(record, evidence); err != nil {
		return nil, err
	}
	resource, resourceVersion, err := e.loadControlRecord(ctx, ticket.session.resourceID())
	if err != nil {
		return nil, err
	}
	if resourceVersion == nil || resource.State != "RESOURCE" || resource.Repository != record.Repository || resource.Generation != record.Generation || resource.ActiveIndex != record.ActiveIndex || resource.Resource == nil || !supportedResourceVersion(resource.Resource.Version) || resource.Resource.Phase != resourceAdopted || resource.Resource.Pending {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "publication resource ownership or protection is unavailable")
	}
	resourceReady, publicationReady := resource, record
	resourceReady.State, publicationReady.State = ports.ProjectionStateReady, ports.ProjectionStateReady
	if !reflect.DeepEqual(metaFromControl(resourceReady), metaFromControl(publicationReady)) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "publication binding differs from its frozen resource metadata")
	}
	if err := validateResourceProtection(resource.Resource); err != nil {
		return nil, err
	}
	found := false
	for _, claim := range resource.Resource.Claims {
		if claim.ID == record.PublicationTicket && (claim.Phase == claimAdmitted || claim.Phase == claimAccepted) && bytes.Equal(claim.Evidence, evidence) {
			found = true
			break
		}
	}
	if !found {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "publication resource no longer holds the admitted reference")
	}
	return e.openPublicationView(ctx, record, *version, ticket, evidence)
}

func (e *openSearchEngine) openPublicationView(ctx context.Context, record controlDoc, version controlVersion, ticket *preparedResource, evidence []byte) (ports.Engine, error) {
	var err error
	record, err = e.restoreResourceCount(ctx, record)
	if err != nil {
		return nil, err
	}
	record.State = ports.ProjectionStateReady
	record.Resource = nil
	record.PublicationVersion, record.PublicationTicket, record.PublicationEvidence = 0, "", nil
	body, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	view := e.resourceScope(e.repository)
	view.primaryShards, view.replicas, view.refreshInterval = e.primaryShards, e.replicas, e.refreshInterval
	view.embedder, view.vectorDimension, view.vectorModel = e.embedder, e.vectorDimension, e.vectorModel
	view.embeddingIdentity = e.embeddingIdentity
	view.fixedControl, view.fixedVersion = body, version
	protection, err := ticket.protectQuery(ctx, evidence)
	if err != nil {
		return nil, err
	}
	view.queryProtection = protection
	meta, err := view.LoadMetaContext(ctx)
	if err != nil {
		_ = view.Close()
		return nil, err
	}
	if meta.Generation != record.Generation || meta.State != ports.ProjectionStateReady {
		_ = view.Close()
		return nil, &missingPublicationResource{kernel.Fail(kernel.ErrCapabilityUnsatisfied, "accepted publication physical generation is unavailable")}
	}
	return view, nil
}
