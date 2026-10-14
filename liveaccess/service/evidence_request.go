package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"kc/kernel"
	"kc/liveaccess/evidence"
	objectmodel "kc/object"
)

// BeginReadEvidence isolates one read's adopted results and supplies a server
// correlation identity when the caller omitted optional diagnostics.
func BeginReadEvidence(ctx context.Context, requestID string) (context.Context, string, error) {
	if requestID == "" {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return ctx, "", kernel.Fail(kernel.ErrTemporaryUnavailable, "allocate read request identity")
		}
		requestID = "read-" + hex.EncodeToString(nonce[:])
	}
	return evidence.WithCollector(ctx, evidence.NewCollector()), requestID, nil
}

// RetainReadEvidence retains only source results present in the delivered
// object set, after the application has applied its existing read policy.
func RetainReadEvidence(ctx context.Context, home string, request objectmodel.RequestContext, delivered []objectmodel.PinnedKnowledgeRef) (*evidence.Reference, error) {
	samples, err := evidence.CapturedValues(ctx)
	if err != nil {
		return nil, err
	}
	adopted := make([]evidence.Sample, 0, len(samples))
	for _, sample := range samples {
		for _, ref := range delivered {
			if sample.Repository == ref.Repository && sample.Commit == ref.Commit && sample.Address.ObjectID == ref.Object {
				adopted = append(adopted, sample)
				break
			}
		}
	}
	if len(adopted) == 0 {
		return nil, nil
	}
	store, err := OpenResultStore(home)
	if err != nil {
		return nil, err
	}
	reference, err := store.Record(ctx, evidence.Record{Identity: request.Identity, Trace: request.Trace, RequestID: request.RequestID, Units: adopted})
	if err != nil {
		return nil, err
	}
	return &reference, nil
}
