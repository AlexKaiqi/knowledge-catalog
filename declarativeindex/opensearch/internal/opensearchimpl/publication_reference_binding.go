package opensearchimpl

import (
	"context"

	"kc/kernel"
)

// An accepted logical publication can be repaired onto a fresh physical
// generation. Its replaced ticket is no longer a consumer root even while
// the Dataset itself remains retained. Independent query/continuation claims
// still protect the old generation. Missing/unknown bindings never prove this.
func (s *rebuildSession) publicationBindingReplaced(ctx context.Context, claim resourceClaim) (bool, error) {
	record, version, err := s.engine.loadControlRecord(ctx, s.engine.publicationBindingID(claim.Evidence))
	if err != nil || version == nil {
		return false, err
	}
	if !validGeneration(record.Generation) || !validGeneration(record.PublicationTicket) {
		return false, kernel.Fail(kernel.ErrPreconditionFailed, "accepted publication mapping has unknown physical identity")
	}
	current := &preparedResource{id: record.PublicationTicket, session: &rebuildSession{engine: s.engine, generation: record.Generation, physicalIndex: s.engine.prefix + "-g-" + record.Generation}}
	if err := current.checkPublicationBinding(record, claim.Evidence); err != nil {
		return false, err
	}
	// Tickets have unique immutable identities: a later repair cannot resurrect
	// this replaced ticket. A reuse of the physical generation needs a new claim.
	return record.Generation != s.generation || record.PublicationTicket != claim.ID, nil
}
