package projection

import (
	"context"
	"errors"
	"strings"
	"testing"

	ports "kc/discovery/projection"
)

type recoverableResourceEngine struct {
	lifecycleEngine
	report ports.ResourceRecoveryReport
	err    error
	calls  int
}

func (e *recoverableResourceEngine) RecoverResources(context.Context) (ports.ResourceRecoveryReport, error) {
	e.calls++
	return e.report, e.err
}

func TestResourceRecoveryContinuesOtherEnginesAndReportsUnknownProtection(t *testing.T) {
	failed := errors.New("provider recovery failed")
	broken := &recoverableResourceEngine{err: failed}
	unknown := &recoverableResourceEngine{report: ports.ResourceRecoveryReport{Retained: []string{"specific-pending-generation"}}}
	idx := &Index{engs: map[engineKey]ports.Engine{
		{repo: "broken"}: broken, {repo: "unknown"}: unknown, {repo: "legacy"}: &lifecycleEngine{},
	}}
	err := idx.RecoverResources(context.Background())
	if !errors.Is(err, failed) || !strings.Contains(err.Error(), "specific-pending-generation") || broken.calls != 1 || unknown.calls != 1 {
		t.Fatalf("recovery lost a provider or unresolved protection: %v, calls=%d/%d", err, broken.calls, unknown.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := idx.RecoverResources(ctx); !errors.Is(err, context.Canceled) || broken.calls != 1 || unknown.calls != 1 {
		t.Fatalf("cancelled recovery entered provider IO: %v, calls=%d/%d", err, broken.calls, unknown.calls)
	}
}

type instanceResourceCoordinator struct {
	recoverableResourceEngine
	publications         int
	closed               int
	computations         int
	computationDirectory string
}

func (r *instanceResourceCoordinator) RecoverPublications(ctx context.Context, resolve func(context.Context, []byte) (bool, error)) (ports.PublicationRecoveryReport, error) {
	r.publications++
	accepted, err := resolve(ctx, []byte("actual-durable-admission"))
	if err != nil {
		return ports.PublicationRecoveryReport{Retained: []string{"specific-resource/ticket"}}, err
	}
	if !accepted {
		return ports.PublicationRecoveryReport{Released: []string{"specific-resource/ticket"}}, nil
	}
	return ports.PublicationRecoveryReport{Confirmed: []string{"specific-resource/ticket"}}, nil
}
func (r *instanceResourceCoordinator) Close() error { r.closed++; return nil }

func TestResourceRecoveryUsesInstanceAuthorityWithoutOpenedQueryScopes(t *testing.T) {
	coordinator := &instanceResourceCoordinator{}
	idx := &Index{engs: map[engineKey]ports.Engine{}}
	resolveCalls := 0
	idx.SetResourceRecovery(coordinator, func(_ context.Context, proof []byte) (bool, error) {
		resolveCalls++
		if string(proof) != "actual-durable-admission" {
			t.Fatalf("recovery resolver lost durable evidence: %q", proof)
		}
		return true, nil
	})
	if err := idx.RecoverResources(context.Background()); err != nil || coordinator.calls != 1 || coordinator.publications != 1 || resolveCalls != 1 {
		t.Fatalf("empty query inventory suppressed resource recovery: %v %+v", err, coordinator)
	}
	// Cached query engines must not duplicate or narrow an instance-wide pass.
	local := &recoverableResourceEngine{}
	idx.engs[engineKey{repo: "query-only"}] = local
	if err := idx.RecoverResources(context.Background()); err != nil || local.calls != 0 || coordinator.calls != 2 {
		t.Fatalf("instance recovery was duplicated or scoped by query handles: %v", err)
	}
	failed := errors.New("publication authority is unavailable")
	idx.SetResourceRecovery(coordinator, func(context.Context, []byte) (bool, error) { return false, failed })
	if err := idx.RecoverResources(context.Background()); !errors.Is(err, failed) || !strings.Contains(err.Error(), "specific-resource/ticket") {
		t.Fatalf("unknown publication protection was hidden: %v", err)
	}
	if coordinator.calls != 3 {
		t.Fatal("unknown publication stopped independent candidate recovery")
	}
}

type referenceResourceCoordinator struct {
	instanceResourceCoordinator
	references     int
	referenceError error
}

func (r *referenceResourceCoordinator) ReconcilePublicationReferences(ctx context.Context, lookup func(context.Context, []byte) (bool, error)) (ports.PublicationRecoveryReport, error) {
	r.references++
	if r.publications != r.references || r.calls != r.references-1 {
		return ports.PublicationRecoveryReport{}, errors.New("reference reconciliation ran outside acceptance/candidate ordering")
	}
	if _, err := lookup(ctx, []byte("accepted-resource-reference")); err != nil {
		return ports.PublicationRecoveryReport{Retained: []string{"accepted-resource-reference"}}, err
	}
	return ports.PublicationRecoveryReport{}, r.referenceError
}
func TestResourceRecoveryReconcilesAcceptedReferencesWithoutQueryScopes(t *testing.T) {
	manager := &referenceResourceCoordinator{}
	idx := &Index{engs: map[engineKey]ports.Engine{}}
	idx.SetResourceRecovery(manager, func(context.Context, []byte) (bool, error) { return true, nil })
	calls := 0
	idx.SetPublicationReferenceLookup(func(_ context.Context, proof []byte) (bool, error) {
		calls++
		if string(proof) != "accepted-resource-reference" {
			t.Errorf("lost exact reference: %q", proof)
		}
		return false, nil
	})
	if err := idx.RecoverResources(context.Background()); err != nil || calls != 1 || manager.references != 1 || manager.calls != 1 {
		t.Fatalf("accepted references were scoped by query inventory: %+v calls=%d %v", manager, calls, err)
	}
	unknown := errors.New("actual lifecycle unavailable")
	idx.SetPublicationReferenceLookup(func(context.Context, []byte) (bool, error) { return false, unknown })
	if err := idx.RecoverResources(context.Background()); !errors.Is(err, unknown) || !strings.Contains(err.Error(), "accepted-resource-reference") || manager.calls != 2 {
		t.Fatalf("unknown eligibility was hidden or blocked independent candidate recovery: %+v %v", manager, err)
	}
}

func (r *instanceResourceCoordinator) RecoverComputations(ctx context.Context, directory string) (ports.ResourceRecoveryReport, error) {
	r.computations++
	r.computationDirectory = directory
	return ports.ResourceRecoveryReport{}, ctx.Err()
}
func TestResourceRecoverySweepsComputationsWithoutAnyOpenedQueries(t *testing.T) {
	coordinator := &instanceResourceCoordinator{}
	idx := &Index{dir: t.TempDir(), engs: map[engineKey]ports.Engine{}}
	idx.SetResourceRecovery(coordinator, nil)
	if err := idx.RecoverResources(context.Background()); err != nil || coordinator.computations != 1 || coordinator.computationDirectory != idx.dir || coordinator.calls != 1 {
		t.Fatalf("computation maintenance depends on query inventory: %+v %v", coordinator, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := idx.RecoverResources(ctx); !errors.Is(err, context.Canceled) || coordinator.computations != 1 {
		t.Fatalf("cancelled sweep entered local computation storage: %+v %v", coordinator, err)
	}
}
