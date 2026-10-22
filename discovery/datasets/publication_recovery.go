package datasets

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"

	"kc/kernel"
	"kc/snapshot/dataset"
)

// PublicationRecovery resolves an already durable admission against the actual
// publication authority. Every resolution establishes its fence after reading
// the admission evidence: a checkpoint cached before admission cannot discharge
// that admission. A failed fence, binding mismatch, or malformed evidence is
// unknown and must retain the resource protection.
type PublicationRecovery struct {
	Authority   dataset.PublicationCoordinator
	Definitions interface {
		DatasetVersion(string, int) (dataset.Dataset, error)
	}
}

func (r PublicationRecovery) Resolve(ctx context.Context, raw []byte) (bool, error) {
	return r.resolve(ctx, raw, false)
}

// Referenced separates current consumer eligibility from historical acceptance.
// Unretired retained definitions remain referenced; known retirement releases
// them. Missing/corrupt authority is unknown, never inferred retirement.
func (r PublicationRecovery) Referenced(ctx context.Context, raw []byte) (bool, error) {
	return r.resolve(ctx, raw, true)
}

func (r PublicationRecovery) resolve(ctx context.Context, raw []byte, live bool) (bool, error) {
	var proof publicationResourceEvidence
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proof); err != nil {
		return false, kernel.Fail(kernel.ErrPreconditionFailed, "publication resource evidence is invalid: %v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return false, kernel.Fail(kernel.ErrPreconditionFailed, "publication resource evidence has trailing content")
	}
	if (proof.Version != 1 && proof.Version != 2) || (proof.Version == 2 && proof.Admission == "") || (proof.Version == 1 && proof.Admission != "") || proof.Authority.AuthorityID == "" || proof.Authority.Binding == "" || proof.Authority.Head == "" ||
		!strings.HasPrefix(proof.Authority.Ref, "refs/heads/") || proof.Definition.SetID == "" || proof.Definition.Revision <= 0 || len(proof.Definition.Sources) == 0 || len(proof.Definition.Items) == 0 || proof.Definition.Retired {
		return false, kernel.Fail(kernel.ErrPreconditionFailed, "publication resource evidence is incomplete or unsupported")
	}
	if r.Authority == nil {
		return false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "publication resource recovery requires its authority")
	}
	lookup := dataset.PublicationLookup{SetID: proof.Definition.SetID, Revision: proof.Definition.Revision}
	var checkpoint dataset.PublicationCheckpoint
	var err error
	if live {
		inspector, ok := r.Authority.(dataset.PublicationInspector)
		if !ok {
			return false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "accepted reference reconciliation requires actual read-only publication inspection")
		}
		checkpoint, err = inspector.InspectPublications(ctx, lookup)
	} else {
		checkpoint, err = r.Authority.FencePublications(ctx, lookup)
	}
	if err != nil {
		return false, err
	}
	actual := checkpoint.Authority
	if actual.AuthorityID != proof.Authority.AuthorityID || actual.Binding != proof.Authority.Binding || actual.Ref != proof.Authority.Ref ||
		actual.Head == "" || actual.Head == proof.Authority.Head || checkpoint.State.AuthorityID != string(actual.AuthorityID) {
		return false, kernel.Fail(kernel.ErrPreconditionFailed, "publication recovery fence does not exclude the admitted authority version")
	}
	accepted, err := resolvePublicationReceipt(proof, checkpoint)
	if err != nil || !accepted || !live {
		return accepted, err
	}
	for _, current := range checkpoint.State.Datasets {
		if current.SetID == proof.Definition.SetID {
			return !current.Retired, nil
		}
	}
	return false, kernel.Fail(kernel.ErrPreconditionFailed, "accepted publication has no current Dataset lifecycle authority")
}

func resolvePublicationReceipt(proof publicationResourceEvidence, checkpoint dataset.PublicationCheckpoint) (bool, error) {
	var definition *dataset.Dataset
	for _, definitions := range [][]dataset.Dataset{checkpoint.State.DatasetVersions, checkpoint.State.Datasets} {
		for _, accepted := range definitions {
			if accepted.SetID == proof.Definition.SetID && accepted.Revision == proof.Definition.Revision {
				accepted.Retired = false
				definition = &accepted
				break
			}
		}
		if definition != nil {
			break
		}
	}
	matching := definition != nil && kernel.CanonicalDigest(*definition) == kernel.CanonicalDigest(proof.Definition)
	var receipt *dataset.PublicationReceipt
	for _, candidate := range checkpoint.Receipts {
		if candidate.SetID == proof.Definition.SetID && candidate.Revision == proof.Definition.Revision {
			receipt = &candidate
			break
		}
	}
	if receipt != nil {
		actual := checkpoint.Authority
		if definition == nil || receipt.Version != 1 || receipt.Admission == "" || receipt.Definition != kernel.CanonicalDigest(*definition) ||
			receipt.Authority.AuthorityID != actual.AuthorityID || receipt.Authority.Binding != actual.Binding || receipt.Authority.Ref != actual.Ref {
			return false, kernel.Fail(kernel.ErrPreconditionFailed, "accepted publication receipt does not match authority state")
		}
	}
	if proof.Version == 1 {
		if matching {
			return false, kernel.Fail(kernel.ErrTemporaryUnavailable, "legacy publication evidence cannot identify the adopted preparation")
		}
		return false, nil
	}
	if receipt == nil {
		if matching {
			return false, kernel.Fail(kernel.ErrTemporaryUnavailable, "accepted publication has no preparation receipt")
		}
		return false, nil
	}
	if receipt.Admission != proof.Admission {
		return false, nil
	}
	if !matching || receipt.Authority != proof.Authority {
		return false, kernel.Fail(kernel.ErrPreconditionFailed, "publication admission receipt has different immutable intent")
	}
	return true, nil
}
