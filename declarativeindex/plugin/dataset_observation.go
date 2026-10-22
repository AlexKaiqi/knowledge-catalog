package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	ports "kc/discovery/projection"
	"kc/kernel"
	"kc/snapshot/dataset"
)

const observationEvidenceKind = "dataset-observation-v1"
const observationContinuationGrace = 2 * time.Minute

type observationAdmission struct {
	Kind       string                       `json:"kind"`
	ID         string                       `json:"id"`
	Owner      string                       `json:"owner"`
	Lookup     dataset.PublicationLookup    `json:"lookup"`
	Definition kernel.Digest                `json:"definition"`
	Authority  dataset.PublicationAuthority `json:"authority"`
}

type retainedObservation struct {
	Evidence []byte `json:"evidence"`
	Until    int64  `json:"until"`
}

type observationReferences struct {
	Version  int                   `json:"version"`
	Current  []byte                `json:"current"`
	Previous []retainedObservation `json:"previous,omitempty"`
}

type datasetObservationAuthority struct {
	references   dataset.RevisionReferenceCoordinator
	publications dataset.PublicationCoordinator
	owner        string
	now          func() time.Time
}

func strictObservationJSON(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "invalid observation authority evidence: %v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return kernel.Fail(kernel.ErrPreconditionFailed, "observation authority evidence has trailing content")
	}
	return nil
}

func observationProof(checkpoint dataset.RevisionReferenceCheckpoint) observationAdmission {
	return observationAdmission{Kind: observationEvidenceKind, ID: checkpoint.ID, Owner: checkpoint.Owner, Lookup: checkpoint.PublicationLookup, Definition: checkpoint.Definition, Authority: checkpoint.PublicationAuthority}
}

func (a datasetObservationAuthority) decode(record dataset.RevisionReference) (observationReferences, error) {
	var references observationReferences
	if err := strictObservationJSON(record.Value, &references); err != nil {
		return references, err
	}
	var proof observationAdmission
	if err := strictObservationJSON(references.Current, &proof); err != nil {
		return references, err
	}
	expected := observationAdmission{Kind: observationEvidenceKind, ID: record.ID, Owner: a.owner, Lookup: record.PublicationLookup, Definition: record.Definition, Authority: record.Authority}
	if references.Version != 1 || record.Owner != a.owner || proof != expected {
		return references, kernel.Fail(kernel.ErrPreconditionFailed, "accepted observation differs from its authority receipt")
	}
	seen := map[string]bool{string(references.Current): true}
	for _, previous := range references.Previous {
		var prior observationAdmission
		if err := strictObservationJSON(previous.Evidence, &prior); err != nil {
			return references, err
		}
		if previous.Until <= 0 || seen[string(previous.Evidence)] || prior.Kind != proof.Kind || prior.ID == "" || prior.Owner != proof.Owner || prior.Lookup != proof.Lookup || prior.Definition != proof.Definition || prior.Authority.AuthorityID != proof.Authority.AuthorityID || prior.Authority.Binding != proof.Authority.Binding || prior.Authority.Ref != proof.Authority.Ref || prior.Authority.Head == "" {
			return references, kernel.Fail(kernel.ErrPreconditionFailed, "retained observation differs from its accepted scope")
		}
		seen[string(previous.Evidence)] = true
	}
	return references, nil
}

func (a datasetObservationAuthority) begin(ctx context.Context, setID string, revision int) (func(func() (ports.PreparedResource, error)) error, error) {
	checkpoint, err := a.references.PrepareRevisionReference(ctx, dataset.PublicationLookup{SetID: setID, Revision: revision}, a.owner)
	if err != nil {
		return nil, err
	}
	// Decode the prior reference before any resource allocation or root changes.
	var prior observationReferences
	if checkpoint.Current != nil {
		prior, err = a.decode(*checkpoint.Current)
		if err != nil {
			return nil, err
		}
	}
	return func(prepare func() (ports.PreparedResource, error)) (err error) {
		ticket, err := prepare()
		if ticket == nil {
			if err != nil {
				return err
			}
			return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "managed observations require a preparation protection ticket")
		}
		attempted := false
		defer func() {
			if !attempted {
				err = errors.Join(err, ticket.Cancel(ctx))
			}
			err = errors.Join(err, ticket.Close())
		}()
		if err != nil {
			return err
		}
		evidence, err := json.Marshal(observationProof(checkpoint))
		if err != nil {
			return err
		}
		attempted = true
		if err := ticket.Admit(ctx, evidence); err != nil {
			return err
		}
		// This timestamp belongs to the authority acceptance attempt, after resource
		// admission; technical ROOT writes cannot start or extend the old window.
		now := a.now()
		next := observationReferences{Version: 1, Current: evidence}
		for _, previous := range prior.Previous {
			if previous.Until > now.UnixNano() {
				next.Previous = append(next.Previous, previous)
			}
		}
		if len(prior.Current) != 0 {
			next.Previous = append(next.Previous, retainedObservation{Evidence: prior.Current, Until: now.Add(observationContinuationGrace).UnixNano()})
		}
		value, err := json.Marshal(next)
		if err != nil {
			return err
		}
		if _, err := a.references.CommitRevisionReference(ctx, checkpoint, value); err != nil {
			return err
		}
		return ticket.Confirm(ctx)
	}, nil
}

func (a datasetObservationAuthority) evidence(ctx context.Context, setID string, revision int) ([]ports.ObservationReference, error) {
	record, err := a.references.RevisionReference(ctx, dataset.PublicationLookup{SetID: setID, Revision: revision}, a.owner)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	references, err := a.decode(record)
	if err != nil {
		return nil, err
	}
	result := []ports.ObservationReference{{Evidence: references.Current}}
	for _, previous := range references.Previous {
		if previous.Until > a.now().UnixNano() {
			result = append(result, ports.ObservationReference{Evidence: previous.Evidence, ContinuationUntil: time.Unix(0, previous.Until)})
		}
	}
	return result, nil
}

func (a datasetObservationAuthority) resolveReference(ctx context.Context, raw []byte, fence bool) (dataset.PublicationCheckpoint, *dataset.RevisionReference, error) {
	var proof observationAdmission
	if err := strictObservationJSON(raw, &proof); err != nil {
		return dataset.PublicationCheckpoint{}, nil, err
	}
	if proof.Kind != observationEvidenceKind || proof.Owner != a.owner || proof.ID == "" || proof.Lookup.SetID == "" || proof.Lookup.Revision <= 0 || proof.Definition == "" || proof.Authority.AuthorityID == "" || proof.Authority.Binding == "" || proof.Authority.Ref == "" || proof.Authority.Head == "" {
		return dataset.PublicationCheckpoint{}, nil, kernel.Fail(kernel.ErrPreconditionFailed, "incomplete observation resource admission")
	}
	// A new actual fence excludes the paused original CAS before absence can
	// release an admission. Receipt lookup alone would race the old publisher.
	var checkpoint dataset.PublicationCheckpoint
	var err error
	if fence {
		checkpoint, err = a.publications.FencePublications(ctx, proof.Lookup)
	} else {
		inspector, ok := a.publications.(dataset.PublicationInspector)
		if !ok {
			return dataset.PublicationCheckpoint{}, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "accepted observation reconciliation requires actual read-only publication inspection")
		}
		checkpoint, err = inspector.InspectPublications(ctx)
	}
	if err != nil {
		return dataset.PublicationCheckpoint{}, nil, err
	}
	actual := checkpoint.Authority
	if actual.AuthorityID != proof.Authority.AuthorityID || actual.Binding != proof.Authority.Binding || actual.Ref != proof.Authority.Ref || actual.Head == "" || actual.Head == proof.Authority.Head || checkpoint.State.AuthorityID != string(actual.AuthorityID) {
		return dataset.PublicationCheckpoint{}, nil, kernel.Fail(kernel.ErrPreconditionFailed, "observation recovery fence differs from its authority")
	}
	receipt, err := a.references.RevisionReferenceReceipt(ctx, proof.ID)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return checkpoint, nil, nil
	}
	if err != nil {
		return dataset.PublicationCheckpoint{}, nil, err
	}
	references, err := a.decode(receipt)
	if err != nil {
		return dataset.PublicationCheckpoint{}, nil, err
	}
	if !bytes.Equal(references.Current, raw) {
		return dataset.PublicationCheckpoint{}, nil, kernel.Fail(kernel.ErrPreconditionFailed, "observation receipt has different immutable admission")
	}
	return checkpoint, &receipt, nil
}

func (a datasetObservationAuthority) resolve(ctx context.Context, raw []byte) (bool, error) {
	_, receipt, err := a.resolveReference(ctx, raw, true)
	return receipt != nil, err
}

func (a datasetObservationAuthority) referenced(ctx context.Context, raw []byte) (bool, error) {
	checkpoint, receipt, err := a.resolveReference(ctx, raw, false)
	if err != nil || receipt == nil {
		return false, err
	}
	found := false
	for _, current := range checkpoint.State.Datasets {
		if current.SetID == receipt.SetID {
			found = true
			if current.Retired {
				return false, nil
			}
			break
		}
	}
	if !found {
		return false, kernel.Fail(kernel.ErrPreconditionFailed, "accepted observation has no current Dataset lifecycle authority")
	}
	record, err := a.references.RevisionReference(ctx, receipt.PublicationLookup, a.owner)
	if err != nil {
		return false, err
	}
	references, err := a.decode(record)
	if err != nil {
		return false, err
	}
	if bytes.Equal(references.Current, raw) {
		return true, nil
	}
	for _, previous := range references.Previous {
		if bytes.Equal(previous.Evidence, raw) && previous.Until > a.now().UnixNano() {
			return true, nil
		}
	}
	return false, nil
}

func routeResourceAuthority(observation, publication func(context.Context, []byte) (bool, error)) func(context.Context, []byte) (bool, error) {
	return func(ctx context.Context, raw []byte) (bool, error) {
		var kind struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(raw, &kind); err != nil {
			return false, kernel.Fail(kernel.ErrPreconditionFailed, "invalid resource authority evidence: %v", err)
		}
		if kind.Kind == observationEvidenceKind {
			return observation(ctx, raw)
		}
		return publication(ctx, raw)
	}
}
