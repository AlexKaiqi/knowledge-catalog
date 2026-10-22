package projection

import "context"

// PreparedResource protects one exact, ready generation. Admit stores opaque
// publication authority evidence durably before the authority CAS is attempted.
// A failed or lost Admit response may already have installed that protection.
// Confirm records known acceptance. Cancel only releases a never-admitted
// preparation; it must reject an admitted or accepted publication. Unknown
// acceptance requires authority recovery, never cancellation by timeout.
type PreparedResource interface {
	Admit(context.Context, []byte) error
	Confirm(context.Context) error
	Cancel(context.Context) error
	// Close releases only the local handle; durable protection remains.
	Close() error
}

// ResourcePreparer is optional for providers with managed physical resources.
// It pins the full expected metadata, not whichever generation is current later.
// Missing ownership or incompatible metadata must fail without adopting legacy
// resources by name. Acquisition and deletion use the same durable exclusion.
type ResourcePreparer interface {
	PrepareResource(context.Context, Meta) (PreparedResource, error)
}

// PreparedRebuildSession commits a new generation with its preparation protection
// already durable. The returned ticket names that generation even if ROOT is
// replaced before the caller resumes. Errors never authorize releasing unknown
// publication protection; unreturned preparations expire through recovery.
type PreparedRebuildSession interface {
	RebuildSession
	CommitResource() (PreparedResource, error)
}

// PublicationRecoveryReport describes durable reference transitions, not
// physical deletions. Retained identifies admissions whose outcome is unknown.
type PublicationRecoveryReport struct {
	More                          bool
	Confirmed, Released, Retained []string
}

// PublicationResourceMaintainer resolves durable admissions through their
// publication authority. Resolve runs outside provider CAS or process locks;
// mutation must recheck the exact claim and evidence afterward.
type PublicationResourceMaintainer interface {
	RecoverPublications(context.Context, func(context.Context, []byte) (bool, error)) (PublicationRecoveryReport, error)
}

// PublicationReferenceMaintainer reconciles accepted references against their
// actual consumer authority. False must prove irreversible loss of this exact
// identity's eligibility, not merely absence from a cache or a failed read.
// Unknown errors retain protection; query/continuation leases are independent.
type PublicationReferenceMaintainer interface {
	ReconcilePublicationReferences(context.Context, func(context.Context, []byte) (bool, error)) (PublicationRecoveryReport, error)
}

// ResourceCoordinator owns instance-wide maintenance independently of opened
// query scopes or source demand. Opening it performs no remote IO.
type ResourceCoordinator interface {
	ResourceMaintainer
	PublicationResourceMaintainer
	Close() error
}

// PublicationBinder opens a read-only view of the exact resource admitted for
// opaque authority evidence. Missing bindings must fail; binding never selects
// a newer generation or performs preparation/recovery. Managed query protection
// is independent of publication; Close releases only this bounded query view.
type PublicationBinder interface {
	BindPublication(context.Context, []byte) (Engine, error)
}

// PublicationRepairer is a maintenance-only handoff. It observes the binding
// before invoking prepare, protects the resulting ready resource, and runs the
// binding CAS through accept's current authority eligibility check. Unknown
// ownership/outcomes retain protection; ordinary queries never call this port.
// Desired metadata carries the current fixed interpretation and physical contract;
// a readable but incompatible generation is not considered repaired.
type PublicationRepairer interface {
	RepairPublication(context.Context, []byte, Meta, func(context.Context) (Meta, error), func(func() error) error) error
}

// PublicationContinuationBinder resolves a provider continuation through its
// durable protection, preserving the original generation across replacement.
// It must reject missing/expired protection and never choose the active root.
type PublicationContinuationBinder interface {
	BindPublicationContinuation(context.Context, []byte, string) (Engine, error)
}

// ComputationMaintainer independently sweeps instance-owned local computation
// copies, including models without query handles. Directory is the instance's
// explicit runtime directory, not a provider-selected filesystem root.
type ComputationMaintainer interface {
	RecoverComputations(context.Context, string) (ResourceRecoveryReport, error)
}
