package datasets

import (
	"context"
	"encoding/json"

	"kc/kernel"
	"kc/snapshot/dataset"
)

// Evidence reconstructs the exact opaque preparation identity committed with
// one fixed Dataset release. An existing legacy release without a receipt
// returns no evidence; unavailable authority or inconsistent metadata fails.
func (r PublicationRecovery) Evidence(ctx context.Context, setID string, revision int) ([]byte, error) {
	if r.Authority == nil || r.Definitions == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "published resource lookup requires its Dataset authority")
	}
	definition, err := r.Definitions.DatasetVersion(setID, revision)
	if err != nil {
		return nil, err
	}
	receipt, err := r.Authority.PublicationReceipt(ctx, dataset.PublicationLookup{SetID: setID, Revision: revision})
	if err != nil {
		return nil, err
	}
	if receipt.Admission == "" {
		return nil, nil
	}
	definition.Retired = false
	if receipt.Version != 1 || receipt.SetID != setID || receipt.Revision != revision || receipt.Definition != kernel.CanonicalDigest(definition) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "publication receipt differs from the fixed Dataset definition")
	}
	return json.Marshal(publicationResourceEvidence{Version: 2, Admission: receipt.Admission, Authority: receipt.Authority, Definition: definition})
}
