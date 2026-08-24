package scenario

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kc/catalog"
	"kc/cli"
	"kc/index"
	"kc/internal/repofile"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/local"
	"kc/reader"
	"kc/repository"
	"kc/writer"
)

// TestUserPublishedKnowledgeAutomaticallyRefreshesIndex proves the user-facing
// publication paths, rather than only Index.Apply in isolation:
//
//	checkout -> edit -> commit --workspace -> AfterSnapshot -> incremental index
//	proposal -> preview -> validate -> merge -> AfterSnapshot -> incremental index
//	checkout -> delete -> commit --workspace -> removal from the index
//
// It also injects an index failure to prove that publication remains canonical,
// lag is explicit, and the next successful Ensure repairs the projection.
func TestUserPublishedKnowledgeAutomaticallyRefreshesIndex(t *testing.T) {
	home := testkit.TempDir(t)
	repoID := "kr://acme/personals/index-author"
	workspaceID := "index-author-desk"

	publishedKC(t, home, "init", "--catalog", "kr://acme/catalog")
	publishedKC(t, home, "repo-add", "--repo", repoID)
	publishedKC(t, home, "put", "--command-id", "schema-note-body", "--repo", repoID,
		"--object", "schema/note.body", "--value", `{"entity":"Note","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`)
	publishedKC(t, home, "put", "--command-id", "seed-retention", "--repo", repoID,
		"--object", "Note:retention", "--value", `{"body":"legacyretention wording"}`)
	publishedKC(t, home, "put", "--command-id", "seed-governed", "--repo", repoID,
		"--object", "Note:governed", "--value", `{"body":"draftgovernance wording"}`)
	publishedKC(t, home, "define-workspace", "--workspace", workspaceID, "--revision", "1",
		"--source", repoID+"=refs/heads/main@")
	publishedAssertSearch(t, home, workspaceID, "legacyretention", "Note:retention")

	checkout := filepath.Join(t.TempDir(), "author-worktree")
	publishedKC(t, home, "checkout", "--workspace", workspaceID, "--to", checkout)
	retentionPath := filepath.Join(checkout, "objects", "Note:retention.json")
	updated, err := repofile.Serialize(
		kernel.Address{Kind: kernel.KindEntity, ObjectID: "Note:retention"},
		"", "", nil, map[string]any{"body": "renewalsignal governed wording"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(retentionPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceCommit := publishedWorkspaceCommit(t, publishedKC(t, home, "commit", "--workspace", workspaceID,
		"--to", checkout, "--command-id", "author-retention-v2", "--message", "revise retention knowledge"))
	publishedAssertSearch(t, home, workspaceID, "renewalsignal", "Note:retention")
	publishedAssertSearch(t, home, workspaceID, "legacyretention")
	publishedAssertIndexBasis(t, home, repoID, workspaceCommit, false)

	if err := os.Remove(retentionPath); err != nil {
		t.Fatal(err)
	}
	deleteCommit := publishedWorkspaceCommit(t, publishedKC(t, home, "commit", "--workspace", workspaceID,
		"--to", checkout, "--command-id", "author-retention-delete", "--message", "remove retired retention knowledge"))
	publishedAssertSearch(t, home, workspaceID, "renewalsignal")
	publishedAssertIndexBasis(t, home, repoID, deleteCommit, false)

	proposal := publishedMap(t, publishedKC(t, home, "propose",
		"--proposal-id", "PR-index-governed", "--repo", repoID,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-index-governed",
		"--object", "Note:governed", "--value", `{"body":"approvedgovernance wording"}`))
	preview := publishedMap(t, publishedKC(t, home, "preview", "--proposal", publishedString(t, proposal, "proposalId"),
		"--workspace", workspaceID))
	validation := publishedMap(t, publishedKC(t, home, "validate", "--preview", publishedString(t, preview, "previewId")))
	publishedKC(t, home, "merge",
		"--proposal", publishedString(t, proposal, "proposalId"),
		"--preview", publishedString(t, preview, "previewId"),
		"--validation", publishedString(t, validation, "reportId"))
	mergeCommit := publishedResolvedCommit(t, home, workspaceID, repoID)
	publishedAssertSearch(t, home, workspaceID, "approvedgovernance", "Note:governed")
	publishedAssertSearch(t, home, workspaceID, "draftgovernance")
	publishedAssertIndexBasis(t, home, repoID, mergeCommit, false)

	failure := publishedIndexFailureRecovery(t)
	report := map[string]any{
		"validation": "user-published-knowledge-auto-index",
		"outcome":    "PASSED",
		"repository": repoID,
		"workspace":  workspaceID,
		"workspaceEdit": map[string]any{
			"commit": workspaceCommit, "newTermVisible": true, "oldTermRemoved": true, "indexCaughtUp": true,
		},
		"workspaceDelete": map[string]any{
			"commit": deleteCommit, "removedTermAbsent": true, "indexCaughtUp": true,
		},
		"governedMerge": map[string]any{
			"commit": mergeCommit, "newTermVisible": true, "oldTermRemoved": true, "indexCaughtUp": true,
		},
		"failureRecovery": failure,
		"assertions": []string{
			"checkout edit and commit --workspace automatically replace the indexed document",
			"the old searchable term disappears when the new term becomes visible",
			"deleting the knowledge file removes the object from search",
			"proposal merge automatically advances the live index to the published commit",
			"an index hook failure does not roll back canonical publication and a later ensure repairs lag",
		},
	}
	publishedWriteEvidence(t, report)
}

type publishedFailEngine struct {
	index.Engine
	fail *bool
}

func (e *publishedFailEngine) Rebuild(docs []index.CompiledDoc, meta index.Meta) error {
	if *e.fail {
		return errors.New("injected index rebuild failure")
	}
	return e.Engine.Rebuild(docs, meta)
}

func (e *publishedFailEngine) Apply(upserts []index.CompiledDoc, deletes []kernel.ObjectID, meta index.Meta) error {
	if *e.fail {
		return errors.New("injected index apply failure")
	}
	return e.Engine.Apply(upserts, deletes, meta)
}

func publishedIndexFailureRecovery(t *testing.T) map[string]any {
	t.Helper()
	repoID := kernel.RepositoryID("kr://acme/validation/index-recovery")
	store := repository.NewStore()
	t.Cleanup(func() { _ = store.Close() })
	repo := testkit.MakeRepository(t, string(repoID))
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := catalog.NewRegistry(testkit.TempDir(t), "kr://acme/validation/index-recovery-catalog")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewCatalog(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository(repoID); err != nil {
		t.Fatal(err)
	}
	fail := false
	idx := index.NewIndexEngine(testkit.TempDir(t), func(dir string, id kernel.RepositoryID) (index.Engine, error) {
		eng, openErr := local.OpenSQLite(dir, id)
		if openErr != nil {
			return nil, openErr
		}
		return &publishedFailEngine{Engine: eng, fail: &fail}, nil
	})
	t.Cleanup(func() { _ = idx.Close() })
	cat.AddHook(declarativeIndexHook{idx: idx})

	initial := declarativeCommit(t, w, "index-recovery-v1", repoID, []repository.Operation{
		declarativeSchema("schema/note.body", "Note", "body", map[string]any{
			"body": declarativeField("string", "text"),
		}),
		declarativeAspect("Note:index-recovery", "body", "schema/note.body", map[string]any{"body": "failureold wording"}),
	})
	before, err := idx.Describe(repo)
	if err != nil || before.BasisCommit != initial || before.LagBehindHead {
		t.Fatalf("initial index: %#v %v", before, err)
	}

	fail = true
	published := declarativeCommit(t, w, "index-recovery-v2", repoID, []repository.Operation{
		declarativeAspect("Note:index-recovery", "body", "schema/note.body", map[string]any{"body": "failurerecovered wording"}),
	})
	lagged, err := idx.Describe(repo)
	if err != nil || lagged.BasisCommit != initial || !lagged.LagBehindHead {
		t.Fatalf("failed hook must leave explicit lag without rolling back publish: %#v %v", lagged, err)
	}

	fail = false
	sync, err := idx.Ensure(repo, published)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := idx.Describe(repo)
	if err != nil || recovered.BasisCommit != published || recovered.LagBehindHead {
		t.Fatalf("recovered index: %#v %v", recovered, err)
	}
	newHits, err := idx.Search(repo, reader.SearchOf(reader.SearchMATCH("failurerecovered")))
	if err != nil || len(newHits) != 1 || newHits[0].Address.ObjectID != "Note:index-recovery" {
		t.Fatalf("recovered term: %#v %v", newHits, err)
	}
	oldHits, err := idx.Search(repo, reader.SearchOf(reader.SearchMATCH("failureold")))
	if err != nil || len(oldHits) != 0 {
		t.Fatalf("stale term survived repair: %#v %v", oldHits, err)
	}
	return map[string]any{
		"publishedCommit":      published,
		"publicationSucceeded": true,
		"lagObserved":          true,
		"recovered":            true,
		"syncMode":             sync.Mode,
		"syncCause":            sync.Cause,
		"newTermVisible":       true,
		"oldTermRemoved":       true,
	}
}

func publishedKC(t *testing.T, home, command string, args ...string) any {
	t.Helper()
	argv := []string{command, "--home", home}
	argv = append(argv, args...)
	result := cli.Run(argv)
	if result.Status != 0 {
		t.Fatalf("kc %s failed: %s", command, result.Stdout)
	}
	var decoded any
	if err := json.Unmarshal([]byte(result.Stdout), &decoded); err != nil {
		t.Fatalf("kc %s returned non-JSON %q: %v", command, result.Stdout, err)
	}
	return decoded
}

func publishedMap(t *testing.T, value any) map[string]any {
	t.Helper()
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("want object, got %#v", value)
	}
	return out
}

func publishedString(t *testing.T, value map[string]any, key string) string {
	t.Helper()
	out, ok := value[key].(string)
	if !ok || out == "" {
		t.Fatalf("missing %s in %#v", key, value)
	}
	return out
}

func publishedWorkspaceCommit(t *testing.T, value any) string {
	t.Helper()
	rows, ok := publishedMap(t, value)["commits"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("want one workspace commit, got %#v", value)
	}
	receipt := publishedMap(t, publishedMap(t, rows[0])["receipt"])
	return publishedString(t, publishedMap(t, receipt["result"]), "newCommit")
}

func publishedResolvedCommit(t *testing.T, home, workspaceID, repoID string) string {
	t.Helper()
	resolved := publishedMap(t, publishedKC(t, home, "resolve", "--workspace", workspaceID))
	repositories := publishedMap(t, resolved["repositories"])
	return publishedString(t, repositories, repoID)
}

func publishedAssertSearch(t *testing.T, home, workspaceID, query string, want ...string) {
	t.Helper()
	raw := publishedKC(t, home, "search", "--workspace", workspaceID, "--query", query)
	hits, ok := raw.([]any)
	if !ok {
		t.Fatalf("search %q returned %#v", query, raw)
	}
	got := make([]string, 0, len(hits))
	for _, hit := range hits {
		got = append(got, publishedString(t, publishedMap(t, publishedMap(t, hit)["address"]), "objectId"))
	}
	if len(got) != len(want) {
		t.Fatalf("search %q: want %v, got %v", query, want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("search %q: want %v, got %v", query, want, got)
		}
	}
}

func publishedAssertIndexBasis(t *testing.T, home, repoID, wantCommit string, wantLag bool) {
	t.Helper()
	desc := publishedMap(t, publishedKC(t, home, "describe-index", "--repo", repoID))
	if publishedString(t, desc, "basisCommit") != wantCommit || desc["lagBehindHead"] != wantLag {
		t.Fatalf("index basis: want commit=%s lag=%v, got %#v", wantCommit, wantLag, desc)
	}
}

func publishedWriteEvidence(t *testing.T, report map[string]any) {
	t.Helper()
	path := os.Getenv("KC_USER_PUBLISHED_INDEX_REPORT")
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("validation evidence: %s", path)
}
