package server

import (
	"encoding/json"
	kchome "kc/home"
	inv "kc/invocation"
	"kc/invoke"
	"os"
	"path/filepath"
	"testing"

	indeximplementation "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type readinessEngine struct {
	meta        index.Meta
	physical    kernel.Digest
	revision    string
	activeCalls int
	metaReads   int
}

func (e *readinessEngine) LoadMeta() (index.Meta, error) {
	e.metaReads++
	return e.meta, nil
}
func (e *readinessEngine) Count() (int, error)           { return 0, nil }
func (e *readinessEngine) Close() error                  { return nil }
func (e *readinessEngine) ProviderID() string            { return "readiness-fixture" }
func (e *readinessEngine) ProviderRevision() string      { return e.revision }
func (e *readinessEngine) PhysicalDigest() kernel.Digest { return e.physical }
func (e *readinessEngine) Rebuild(_ []index.CompiledDoc, meta index.Meta) error {
	e.activeCalls++
	e.meta = meta
	return nil
}
func (e *readinessEngine) Apply(_ []index.CompiledDoc, _ []objectmodel.ObjectID, meta index.Meta) error {
	e.activeCalls++
	e.meta = meta
	return nil
}
func (e *readinessEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) index.Capability {
	e.activeCalls++
	return index.Capability{Guarantee: index.GuaranteeExact, Coverage: 1}
}
func (e *readinessEngine) Retrieve(index.RetrieveRequest) (index.CandidatePage, error) {
	e.activeCalls++
	return index.CandidatePage{Exhausted: true}, nil
}

func repositoryReadinessFixture(t *testing.T) (*inv.Invocation, kchome.ManagedRepositoryResult, *readinessEngine) {
	t.Helper()
	repo := testkit.MakeRepository(t, "kr://kaiqidong/notes")
	store := snapshot.NewRegistry()
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	eng := &readinessEngine{physical: "physical-v1", revision: "provider-v1"}
	idx := indeximplementation.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (index.Engine, error) { return eng, nil })
	t.Cleanup(func() { _ = idx.Close() })
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	eng.activeCalls = 0
	eng.metaReads = 0
	dir := t.TempDir()
	if err := inv.WriteAllow(dir, inv.AllowFile{Version: inv.AllowVersion, Rules: []inv.AllowRule{{ID: "projection", Principal: "kaiqidong", Repo: string(repo.ID()), Actions: []string{"projection.read"}}}}); err != nil {
		t.Fatal(err)
	}
	cx := &inv.Invocation{Home: dir, WS: &kchome.Home{Reader: reader.NewReader(store), Index: idx}, Flags: map[string]invoke.FlagValue{"as": "kaiqidong"}, Observation: &invoke.ObservationTelemetry{}}
	return cx, kchome.ManagedRepositoryResult{RepositoryID: string(repo.ID())}, eng
}

func TestRepositoryReadinessVerifiesFixedProjectionAndLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*readinessEngine)
		want   string
		code   kernel.ErrorCode
	}{
		{"valid ready", func(*readinessEngine) {}, "READY", ""},
		{"stale logical declaration", func(e *readinessEngine) { e.meta.AccessDigest = "stale" }, "UNAVAILABLE", kernel.ErrPreconditionFailed},
		{"stale physical mapping", func(e *readinessEngine) { e.physical = "physical-v2" }, "UNAVAILABLE", kernel.ErrPreconditionFailed},
		{"stale provider revision", func(e *readinessEngine) { e.revision = "provider-v2" }, "UNAVAILABLE", kernel.ErrPreconditionFailed},
		{"wrong fixed basis", func(e *readinessEngine) { e.meta.Basis = "another-commit" }, "UNAVAILABLE", kernel.ErrPreconditionFailed},
		{"building", func(e *readinessEngine) { e.meta.State = index.ProjectionStateBuilding }, "BUILDING", kernel.ErrTemporaryUnavailable},
		{"updating", func(e *readinessEngine) { e.meta.State = index.ProjectionStateUpdating }, "UPDATING", kernel.ErrTemporaryUnavailable},
		{"failed", func(e *readinessEngine) { e.meta.State = index.ProjectionStateFailed }, "FAILED", kernel.ErrCapabilityUnsatisfied},
		{"retired", func(e *readinessEngine) { e.meta.State = index.ProjectionStateRetired }, "RETIRED", kernel.ErrCapabilityUnsatisfied},
		{"absent", func(e *readinessEngine) { e.meta = index.Meta{} }, "NOT_READY", kernel.ErrCapabilityUnsatisfied},
		{"unknown lifecycle", func(e *readinessEngine) { e.meta.State = "UNKNOWN" }, "UNAVAILABLE", kernel.ErrCapabilityUnsatisfied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cx, item, eng := repositoryReadinessFixture(t)
			published := eng.meta.Basis
			tc.change(eng)
			out := describeRepositoryReadiness(cx, item)
			if out.Search != tc.want || out.Reason != tc.code {
				t.Fatalf("readiness=%+v; want %s / %s", out, tc.want, tc.code)
			}
			if out.Publication != "PUBLISHED" || out.PublishedCommit != published {
				t.Fatalf("projection failure changed publication: %+v", out)
			}
			if eng.activeCalls != 0 {
				t.Fatalf("status queried or changed the index: %d active calls", eng.activeCalls)
			}
		})
	}
}

func TestRepositoryReadinessOmitsInventoryDescription(t *testing.T) {
	cx, item, _ := repositoryReadinessFixture(t)
	out := describeRepositoryReadiness(cx, item)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["profile"]; ok {
		t.Fatalf("readiness must not keep a profile status word: %s", raw)
	}
	if _, ok := payload["title"]; ok {
		t.Fatalf("readiness must not flatten README into title: %s", raw)
	}
	if _, ok := payload["summary"]; ok {
		t.Fatalf("readiness must not flatten README into summary: %s", raw)
	}
}

func TestRepositoryReadinessSeparatesDeniedFromBrokenPolicy(t *testing.T) {
	cx, item, eng := repositoryReadinessFixture(t)
	if err := inv.WriteAllow(cx.Home, inv.AllowFile{Version: inv.AllowVersion}); err != nil {
		t.Fatal(err)
	}
	out := describeRepositoryReadiness(cx, item)
	if out.Search != "NOT_AUTHORIZED" {
		t.Fatalf("missing projection permission: %+v", out)
	}
	if err := os.WriteFile(filepath.Join(cx.Home, "allow.json"), []byte("invalid policy"), 0600); err != nil {
		t.Fatal(err)
	}
	out = describeRepositoryReadiness(cx, item)
	if out.Search != "UNAVAILABLE" || out.Reason == "" {
		t.Fatalf("policy failure falsely asks for more grants: %+v", out)
	}
	if eng.activeCalls != 0 || eng.metaReads != 0 {
		t.Fatalf("denied status touched the index: %d active calls, %d metadata reads", eng.activeCalls, eng.metaReads)
	}
}
