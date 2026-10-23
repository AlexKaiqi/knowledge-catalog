package datasets

import "context"

// trackedPublicationProtection is the ticket shape a build hands to
// PublicationRecovery.Adopt: it counts how the adoption wrapper drove one build
// claim. Publication itself never holds a ticket, so only adoption tests use it.
type trackedPublicationProtection struct {
	admissionResource
	admitErr, confirmErr                             error
	admissions, confirmations, cancellations, closes int
}

func (r *trackedPublicationProtection) Admit(ctx context.Context, raw []byte) error {
	r.admissions++
	if r.admitErr != nil {
		return r.admitErr
	}
	return r.admissionResource.Admit(ctx, raw)
}
func (r *trackedPublicationProtection) Confirm(context.Context) error {
	r.confirmations++
	return r.confirmErr
}
func (r *trackedPublicationProtection) Cancel(context.Context) error { r.cancellations++; return nil }
func (r *trackedPublicationProtection) Close() error                 { r.closes++; return nil }

var _ PublicationProtection = (*trackedPublicationProtection)(nil)
