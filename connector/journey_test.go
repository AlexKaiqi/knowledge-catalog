package connector_test

import (
	"testing"

	"kc/catalog"
	"kc/connector"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
)

// TestConnectorChangeJourney is the generic J8 contract. The source client and
// source-key translation stay outside this repository; from Desired onward the
// complete preview -> Writer -> Workspace -> consumer path is exercised here.
func TestConnectorChangeJourney(t *testing.T) {
	s := testkit.NewSetup(t, "kr://acme/public/source-mirror")
	cat := testkit.OpenCatalog(t, s.Store)
	if _, err := cat.DefineWorkspace("source-agent", 1, []catalog.WorkspaceSource{{
		Repository: s.RepositoryID,
		Selector:   snapshot.DefaultRef,
	}}); err != nil {
		t.Fatal(err)
	}

	a := structure("Table:source.a")
	b := structure("Table:source.b")
	c := structure("Table:source.c")
	v1a := map[string]any{"name": "a", "version": "V1"}
	v1b := map[string]any{"name": "b", "version": "V1"}

	initial := mustPreview(t, connector.Plan{
		ConnectorID:      "source-structure",
		Mode:             connector.ModeReconcile,
		Scope:            connector.Scope{Aspects: []string{"structure"}},
		TargetRepository: s.RepositoryID,
		BaseCommit:       s.RootCommitID,
		Desired: []connector.Unit{
			{Address: a, Value: v1a, SourceKey: "source/a"},
			{Address: b, Value: v1b, SourceKey: "source/b"},
		},
		SourceRefs: []string{"source://snapshot/S1"},
		ProducedAt: "2026-08-23T01:00:00Z",
	})
	first := mustConnectorCommit(t, s.Writer, "source-structure:S1", initial)
	p0 := mustResolve(t, cat, "source-agent")
	old := reader.Open(reader.Lookup(cat.Require), testkit.WorkspacePin(p0))
	assertVersion(t, old, a.ObjectID, "V1")
	assertSource(t, old, a.ObjectID, "source://snapshot/S1")

	v2a := map[string]any{"name": "a", "version": "V2"}
	v2c := map[string]any{"name": "c", "version": "V2"}
	changed := mustPreview(t, connector.Plan{
		ConnectorID:      "source-structure",
		Mode:             connector.ModeReconcile,
		Scope:            connector.Scope{Aspects: []string{"structure"}},
		TargetRepository: s.RepositoryID,
		BaseCommit:       first.Result.CommitID,
		Desired: []connector.Unit{
			{Address: a, Value: v2a, SourceKey: "source/a"},
			{Address: c, Value: v2c, SourceKey: "source/c"},
		},
		Observed: []connector.Observed{
			{Address: a, Digest: kernel.CanonicalDigest(v1a)},
			{Address: b, Digest: kernel.CanonicalDigest(v1b)},
		},
		SourceRefs: []string{"source://snapshot/S2"},
		ProducedAt: "2026-08-23T02:00:00Z",
	})
	if changed.Summary.Added != 1 || changed.Summary.Updated != 1 || changed.Summary.Removed != 1 {
		t.Fatalf("change summary = %#v", changed.Summary)
	}
	cmd := connector.CommandID("source-structure", "S2")
	second := mustConnectorCommit(t, s.Writer, cmd, changed)
	replayed, err := s.Writer.Commit(cmd, changed.ChangeSet)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Disposition != writer.DispositionReplayed || replayed.Result.CommitID != second.Result.CommitID {
		t.Fatalf("replay = %#v", replayed)
	}

	// An unchanged pull is a no-op before Writer, hence cannot create a commit.
	headBeforeEmpty := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
	empty := mustPreview(t, connector.Plan{
		ConnectorID:      "source-structure",
		Mode:             connector.ModeReconcile,
		Scope:            connector.Scope{Aspects: []string{"structure"}},
		TargetRepository: s.RepositoryID,
		BaseCommit:       headBeforeEmpty,
		Desired:          []connector.Unit{{Address: a, Value: v2a}, {Address: c, Value: v2c}},
		Observed: []connector.Observed{
			{Address: a, Digest: kernel.CanonicalDigest(v2a)},
			{Address: c, Digest: kernel.CanonicalDigest(v2c)},
		},
		SourceRefs: []string{"source://snapshot/S2-retry"},
	})
	if !empty.Empty || len(empty.ChangeSet.Operations) != 0 {
		t.Fatalf("empty preview = %#v", empty)
	}
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != headBeforeEmpty {
		t.Fatalf("empty preview moved head: %s -> %s", headBeforeEmpty, got)
	}

	// The old task remains on S1; a new task resolves S2 and sees add/update/remove.
	assertVersion(t, old, a.ObjectID, "V1")
	assertVersion(t, old, b.ObjectID, "V1")
	livePin := mustResolve(t, cat, "source-agent")
	live := reader.Open(reader.Lookup(cat.Require), testkit.WorkspacePin(livePin))
	assertVersion(t, live, a.ObjectID, "V2")
	assertVersion(t, live, c.ObjectID, "V2")
	if values, err := live.Read(b.ObjectID, nil); err != nil || len(values) != 0 {
		t.Fatalf("removed source object is still visible: %#v %v", values, err)
	}
	assertSource(t, live, a.ObjectID, "source://snapshot/S2")

	// Preview validation failures are pure: neither scope escape nor a missing
	// SOURCE reference can move the knowledge.
	headBeforeRejects := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
	badScope := connector.Plan{
		ConnectorID: "source-structure", Mode: connector.ModePatch,
		Scope:            connector.Scope{Aspects: []string{"structure"}},
		TargetRepository: s.RepositoryID, BaseCommit: headBeforeRejects,
		Desired:    []connector.Unit{{Address: classification("Table:source.a"), Value: map[string]any{"pii": true}}},
		SourceRefs: []string{"source://snapshot/bad"},
	}
	_, err = connector.Preview(badScope)
	testkit.ExpectCode(t, err, kernel.ErrScopeDenied)
	badScope.Desired = []connector.Unit{{Address: a, Value: v2a}}
	badScope.SourceRefs = nil
	_, err = connector.Preview(badScope)
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != headBeforeRejects {
		t.Fatalf("rejected previews moved head: %s -> %s", headBeforeRejects, got)
	}

	// A preview based on S2 cannot overwrite an intervening writer. This is the
	// failure boundary between the external pull and the authoritative COMMIT.
	stale := mustPreview(t, connector.Plan{
		ConnectorID: "source-structure", Mode: connector.ModePatch,
		Scope:            connector.Scope{Aspects: []string{"structure"}},
		TargetRepository: s.RepositoryID, BaseCommit: headBeforeRejects,
		Desired:    []connector.Unit{{Address: a, Value: map[string]any{"name": "a", "version": "STALE"}}},
		Observed:   []connector.Observed{{Address: a, Digest: kernel.CanonicalDigest(v2a)}},
		SourceRefs: []string{"source://snapshot/stale"},
	})
	external, err := s.Writer.Commit("external-between-pull-and-commit", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: headBeforeRejects, ExpectedTargetCommit: headBeforeRejects,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: structure("Table:source.external"),
			Value: map[string]any{"version": "EXTERNAL"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Writer.Commit("source-structure:stale", stale.ChangeSet)
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != external.Result.CommitID {
		t.Fatalf("stale connector partially wrote: head=%s external=%s", got, external.Result.CommitID)
	}
	latest := reader.Open(reader.Lookup(cat.Require), testkit.WorkspacePin(mustResolve(t, cat, "source-agent")))
	assertVersion(t, latest, a.ObjectID, "V2")
}

func mustPreview(t *testing.T, plan connector.Plan) connector.PreviewResult {
	t.Helper()
	preview, err := connector.Preview(plan)
	if err != nil {
		t.Fatal(err)
	}
	return preview
}

func mustConnectorCommit(t *testing.T, w *writer.Writer, commandID string, preview connector.PreviewResult) writer.CommitReceipt {
	t.Helper()
	if preview.Empty {
		t.Fatal("unexpected empty connector preview")
	}
	receipt, err := w.Commit(commandID, preview.ChangeSet)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func mustResolve(t *testing.T, cat *catalog.Catalog, workspaceID string) catalog.ResolvedWorkspace {
	t.Helper()
	resolved, err := cat.ResolveWorkspace(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func assertVersion(t *testing.T, serving *reader.Serving, objectID knowledge.ObjectID, want string) {
	t.Helper()
	values, err := serving.Read(objectID, nil)
	if err != nil || len(values) != 1 {
		t.Fatalf("read %s = %#v, %v", objectID, values, err)
	}
	body, ok := values[0].Value.(map[string]any)
	if !ok {
		t.Fatalf("read %s value = %#v", objectID, values[0].Value)
	}
	structure, ok := body["structure"].(map[string]any)
	if !ok || structure["version"] != want {
		t.Fatalf("read %s version = %#v, want %s", objectID, structure, want)
	}
}

func assertSource(t *testing.T, serving *reader.Serving, objectID knowledge.ObjectID, want string) {
	t.Helper()
	traces, err := serving.GetProvenance(objectID)
	if err != nil || len(traces) != 1 || len(traces[0].Chain) == 0 {
		t.Fatalf("provenance %s = %#v, %v", objectID, traces, err)
	}
	refs := traces[0].Chain[0].SourceRefs
	if len(refs) != 1 || refs[0] != want {
		t.Fatalf("source refs %s = %#v, want %s", objectID, refs, want)
	}
}
