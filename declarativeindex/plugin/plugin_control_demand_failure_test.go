package plugin_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	retrieval "kc/discovery"
	discoverydatasets "kc/discovery/datasets"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

func TestIndexControlDemandReadFailurePreservesEntireVerifiedPartition(t *testing.T) {
	for _, authority := range []string{"inventory", "serving-bases"} {
		t.Run(authority, func(t *testing.T) {
			f := newControlRecoveryFixture(t)
			a, b := f.base.setups[0], f.base.setups[1]
			atA, atB := testkit.MustHead(t, a.Repo, snapshot.DefaultRef), testkit.MustHead(t, b.Repo, snapshot.DefaultRef)
			runtime := f.open(t, a.RepositoryID, b.RepositoryID)
			f.publish(t, runtime, "verified-a", a.RepositoryID, atA)
			f.publish(t, runtime, "verified-b", b.RepositoryID, atB)
			if err := runtime.plugins.CatchUp(context.Background()); err != nil {
				t.Fatal(err)
			}
			nextB := f.advance(t, 1, "unaccepted-c")
			f.discardFixed(t, a.RepositoryID, atA)
			f.discardFixed(t, b.RepositoryID, atB)
			evidence := f.accepted(t)
			fault := kernel.Fail(kernel.ErrTemporaryUnavailable, "effective demand authority unavailable")
			if authority == "inventory" {
				runtime.controller.SetInventory(func() ([]kernel.RepositoryID, error) { return nil, fault })
			} else {
				// The proposed B update arrives before A fails. Neither the
				// incomplete partition nor an unaccepted fixed basis may leak.
				runtime.controller.SetInventory(func() ([]kernel.RepositoryID, error) {
					return []kernel.RepositoryID{b.RepositoryID, a.RepositoryID}, nil
				})
				runtime.controller.SetServingBases(func(id kernel.RepositoryID) ([]kernel.CommitID, error) {
					if id == b.RepositoryID {
						return []kernel.CommitID{nextB}, nil
					}
					return nil, fault
				})
			}
			if err := runtime.plugins.CatchUp(context.Background()); !errors.Is(err, fault) {
				t.Errorf("failed demand refresh hid its original authority error: %v", err)
			}
			f.assertSnapshot(t, runtime, a.RepositoryID, atA, "runbook source 1 at a")
			f.assertSnapshot(t, runtime, b.RepositoryID, atB, "runbook source 2 at a")
			f.mu.Lock()
			for _, build := range f.builds {
				if build.Meta.Basis == nextB && strings.HasSuffix(string(build.Repository), "@"+string(nextB)) {
					t.Errorf("failed demand refresh leaked a partially proposed fixed basis: %#v", build)
				}
			}
			f.mu.Unlock()
			f.assertAccepted(t, evidence)
			runtime.controller.SetInventory(func() ([]kernel.RepositoryID, error) {
				return []kernel.RepositoryID{a.RepositoryID, b.RepositoryID}, nil
			})
			runtime.controller.SetServingBases(discoverydatasets.ServingBasisDemand{Datasets: f.base.cat}.Bases)
			if err := runtime.plugins.CatchUp(context.Background()); err != nil {
				t.Fatalf("demand authority recovery did not resume actual maintenance: %v", err)
			}
			f.assertAccepted(t, evidence)
		})
	}
}

func TestIndexControlInitialDemandFailureDoesNotActivateAndClosesPreparedResources(t *testing.T) {
	for _, authority := range []string{"inventory", "serving-bases"} {
		t.Run(authority, func(t *testing.T) {
			f := newControlRecoveryFixture(t)
			source := f.base.setups[0]
			at := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
			runtime := f.open(t, source.RepositoryID)
			// This is a real explicit preparation before background activation;
			// it makes resource release observable without doing I/O in Entry.Open.
			f.publish(t, runtime, "prepared-before-start", source.RepositoryID, at)
			fault := kernel.Fail(kernel.ErrTemporaryUnavailable, "initial demand authority unavailable")
			if authority == "inventory" {
				runtime.controller.SetInventory(func() ([]kernel.RepositoryID, error) { return nil, fault })
			} else {
				runtime.controller.SetServingBases(func(kernel.RepositoryID) ([]kernel.CommitID, error) { return nil, fault })
			}
			evidence := f.accepted(t)
			f.mu.Lock()
			beforeBuilds := len(f.builds)
			f.mu.Unlock()
			if err := runtime.plugins.Start(context.Background()); !errors.Is(err, fault) {
				t.Fatalf("incomplete initial demand was activated: %v", err)
			}
			if runtime.plugins.Running() {
				t.Error("failed initial demand left a running plugin")
			}
			f.mu.Lock()
			if len(f.builds) != beforeBuilds {
				t.Errorf("failed initial demand performed background physical work: before=%d after=%d", beforeBuilds, len(f.builds))
			}
			f.mu.Unlock()
			assertPluginEngineCloseCounts(t, f.base.pool, 1)
			repo, err := f.base.lookup(source.RepositoryID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.plugins.SearchAtContext(context.Background(), repo, at, retrieval.SearchOf(retrieval.SearchMATCH("runbook"))); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
				t.Errorf("failed activation retained a usable actual contribution: %v", err)
			}
			f.assertAccepted(t, evidence)
		})
	}
}
