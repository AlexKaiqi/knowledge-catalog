package projection_test

import (
	"context"
	"testing"

	"kc/declarativeindex/internal/testfixture"
	index "kc/declarativeindex/projection"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// TestEnsureAtRebuildsWhenPhysicalConfigurationChanges guards the boundary of
// the metadata-only readiness check. A READY projection at the same basis is
// only reusable while the running physical configuration (provider revision and
// declaration manifest digest) is unchanged; when it moves, the stored contract
// cannot be assumed valid and a rebuild must happen.
func TestEnsureAtRebuildsWhenPhysicalConfigurationChanges(t *testing.T) {
	repo, commit := seededRepository(t)
	engine := &testfixture.StateEngine{}
	idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })

	idx.SetDeclarationManifest(nil, kernel.CanonicalDigest("manifest/v1"))
	first, err := idx.EnsureAt(context.Background(), repo, commit)
	if err != nil {
		t.Fatal(err)
	}
	if first.Mode != ports.IndexModeRebuild {
		t.Fatalf("first EnsureAt must build: %+v", first)
	}

	// Same basis, same manifest: the projection is reusable from metadata.
	again, err := idx.EnsureAt(context.Background(), repo, commit)
	if err != nil {
		t.Fatal(err)
	}
	if again.Mode != ports.IndexModeReady {
		t.Fatalf("unchanged configuration must answer READY: %+v", again)
	}

	// The manifest moves: the retained digest is no longer this instance's
	// contract, so readiness cannot be inferred from metadata.
	idx.SetDeclarationManifest(nil, kernel.CanonicalDigest("manifest/v2"))
	rebuilt, err := idx.EnsureAt(context.Background(), repo, commit)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Mode != ports.IndexModeRebuild {
		t.Fatalf("changed physical configuration must rebuild, got %+v", rebuilt)
	}
	if engine.Meta.AccessDigest == "" {
		t.Fatal("rebuild left no access digest on the projection")
	}
}

// TestDatasetPrepareReusesReadyRevisionWithoutReferrerWalk pins the Dataset side
// of the same property: an already prepared revision is answered from metadata,
// so preparing the same pin twice does not re-resolve every declaration
// reference in its sources.
func TestDatasetPrepareReusesReadyRevisionWithoutReferrerWalk(t *testing.T) {
	repo, commit := seededRepository(t)
	counted := &countingReferrerRepository{ObjectRepository: repo}
	eng := &testfixture.StateEngine{}
	idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return eng, nil })
	t.Cleanup(func() { _ = idx.Close() })

	pin := reader.DatasetPin{
		Temporary:    true,
		SetID:        "probe-set",
		Revision:     1,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit},
		Items:        []reader.DatasetItem{{Repository: repo.ID(), Commit: commit, Target: "article/a"}},
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id != repo.ID() {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "unexpected source %s", id)
		}
		return counted, nil
	}

	built, err := idx.PrepareDataset(context.Background(), lookup, pin)
	if err != nil {
		t.Fatal(err)
	}
	if built.BasisCommit == "" {
		t.Fatalf("first prepare produced no basis: %+v", built)
	}

	counted.reset()
	again, err := idx.PrepareDataset(context.Background(), lookup, pin)
	if err != nil {
		t.Fatal(err)
	}
	if again.Mode != ports.IndexModeReady {
		t.Fatalf("second prepare must reuse the ready revision: %+v", again)
	}
	if referrer, reads := counted.counts(); referrer != 0 || reads != 0 {
		t.Fatalf("reused revision re-walked the referrer index: enumerations=%d address reads=%d", referrer, reads)
	}
}
