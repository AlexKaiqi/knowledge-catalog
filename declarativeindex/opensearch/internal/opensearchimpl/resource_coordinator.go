package opensearchimpl

import (
	"context"

	ports "kc/discovery/projection"
)

type resourceCoordinator struct{ client *openSearchEngine }

// OpenResources constructs instance-wide maintenance for an explicitly owned
// namespace. The empty legacy namespace has no exclusive instance ownership;
// callers retain scope-local maintenance until its resources are migrated.
// Neither construction nor Close accesses the backend or creates query scopes.
func OpenResources(cfg Config) (ports.ResourceCoordinator, error) {
	client, err := openConfiguredClient(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Namespace == "" {
		return nil, nil
	}
	return &resourceCoordinator{client: client}, nil
}

func (r *resourceCoordinator) RecoverResources(ctx context.Context) (ports.ResourceRecoveryReport, error) {
	return r.client.recoverCandidates(ctx, true)
}

func (r *resourceCoordinator) RecoverPublications(ctx context.Context, resolve func(context.Context, []byte) (bool, error)) (ports.PublicationRecoveryReport, error) {
	return r.client.recoverPublications(ctx, resolve, true, claimAdmitted)
}
func (*resourceCoordinator) Close() error { return nil }

func (r *resourceCoordinator) ReconcilePublicationReferences(ctx context.Context, referenced func(context.Context, []byte) (bool, error)) (ports.PublicationRecoveryReport, error) {
	return r.client.recoverPublications(ctx, referenced, true, claimAccepted)
}
