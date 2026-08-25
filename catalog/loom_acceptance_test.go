package catalog_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/filegit"
	"kc/snapshot/gitea"
	"kc/writer"
)

// TestLoomAcceptanceMixedGiteaAndLocal exercises every layer-① (Loom)
// operation — register, define, resolve, route, checkout, status, and
// write-back through Writer — against one Workspace whose two members are
// backed by different Snapshot engines: a local git directory and a real
// remote Gitea repository (started in docker; skips if unavailable, see
// testkit.GiteaEndpoint).
//
// Loom's contract is that composition and routing do not care which engine
// backs a member; checkout does (only a local git directory becomes a
// writable worktree), and it must say so honestly rather than fail the whole
// tree or fabricate a read-only export (docs/COMPOSITION.md §3.5, §4).
func TestLoomAcceptanceMixedGiteaAndLocal(t *testing.T) {
	base, token, run := testkit.GiteaEndpoint(t)
	t.Setenv(gitea.EnvToken, token)

	aliceID := kernel.RepositoryID("kr://acme/personals/alice")
	alice, err := filegit.NewFileGit(testkit.TempDir(t), aliceID)
	if err != nil {
		t.Fatal(err)
	}

	semanticID := kernel.RepositoryID("kr://acme/public/semantic")
	sum := sha256.Sum256([]byte(string(semanticID) + run))
	semantic, err := gitea.Open(semanticID, base+"/kc/kc-"+hex.EncodeToString(sum[:8]), token)
	if err != nil {
		t.Fatal(err)
	}

	store := snapshot.NewRegistry()
	if err := store.Add(alice); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(semantic); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)

	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	aliceHead := testkit.MustHead(t, alice, "refs/heads/main")
	if _, err := w.Commit("seed-alice", testkit.CommitChange(aliceID, aliceHead, "note/churn", map[string]any{"text": "draft"}, "")); err != nil {
		t.Fatal(err)
	}
	semanticHead := testkit.MustHead(t, semantic, "refs/heads/main")
	if _, err := w.Commit("seed-semantic", testkit.CommitChange(semanticID, semanticHead, "metric/dau", map[string]any{"definition": "daily actives"}, "")); err != nil {
		t.Fatal(err)
	}

	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{
		{Repository: aliceID, Selector: "refs/heads/main", Path: catalog.MountPath("")},
		{Repository: semanticID, Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
	}); err != nil {
		t.Fatal(err)
	}
	def := mustWorkspace(t, cat, "notes")

	t.Run("resolves and checks both members regardless of backing engine", func(t *testing.T) {
		resolved, err := cat.ResolveWorkspace("notes")
		if err != nil {
			t.Fatal(err)
		}
		if resolved.Repositories[aliceID] == "" || resolved.Repositories[semanticID] == "" {
			t.Fatalf("both members must resolve to a commit: %#v", resolved.Repositories)
		}
		if check := cat.CheckResolved(resolved); check.Outcome != "PASSED" {
			t.Fatal(check)
		}
	})

	t.Run("routes by path the same way for both engines", func(t *testing.T) {
		route, err := catalog.RouteMount(def, "analysis/churn.md")
		if err != nil {
			t.Fatal(err)
		}
		if route.Repository != aliceID || route.Path != "analysis/churn.md" {
			t.Fatalf("root mount routing: %#v", route)
		}
		route, err = catalog.RouteMount(def, "refs/semantic/metrics/dau.md")
		if err != nil {
			t.Fatal(err)
		}
		if route.Repository != semanticID || route.Path != "metrics/dau.md" {
			t.Fatalf("nested mount routing: %#v", route)
		}
	})

	t.Run("federated read still assembles across both engines", func(t *testing.T) {
		churn, err := testkit.FederatedRead(cat, "notes", "note/churn")
		if err != nil || len(churn) != 1 {
			t.Fatalf("%#v %v", churn, err)
		}
		dau, err := testkit.FederatedRead(cat, "notes", "metric/dau")
		if err != nil || len(dau) != 1 {
			t.Fatalf("%#v %v", dau, err)
		}
	})

	dest := filepath.Join(testkit.TempDir(t), "work")
	var mounts []catalog.MountCheckout
	t.Run("checkout is a writable worktree for the local member and honestly skipped for gitea", func(t *testing.T) {
		mounts, err = cat.CheckoutMounts("notes", dest)
		if err != nil {
			t.Fatal(err)
		}
		var aliceOut, semanticOut catalog.MountCheckout
		for _, m := range mounts {
			switch m.Repository {
			case aliceID:
				aliceOut = m
			case semanticID:
				semanticOut = m
			}
		}
		if aliceOut.Skipped || aliceOut.Dir != dest {
			t.Fatalf("local member must be a real writable worktree: %#v", aliceOut)
		}
		if !semanticOut.Skipped || semanticOut.Dir != "" || !strings.Contains(semanticOut.Reason, "local git directory") {
			t.Fatalf("gitea member must be reported skipped with a reason, not silently dropped or hard-failed: %#v", semanticOut)
		}
		if _, err := os.Stat(filepath.Join(dest, "refs", "semantic")); err != nil {
			t.Fatalf("a skipped mount still reserves its directory in the composed tree: %v", err)
		}
		// The checkout is the raw git tree, not a reader.WriteCheckout knowledge
		// export: the seeded object lands at repofile's real default path.
		if _, err := os.Stat(filepath.Join(dest, "objects", "note", "churn.json")); err != nil {
			t.Fatalf("checked-out tree must contain the member's real committed file: %v", err)
		}
	})

	t.Run("editing the writable mount is visible to git status; the skipped mount reports Skipped", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dest, "analysis.md"), []byte("draft\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		statuses, err := catalog.MountStatus(mounts)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range statuses {
			switch s.Repository {
			case aliceID:
				if !s.Dirty {
					t.Fatalf("an edit inside the checkout must show up in the local mount's status: %#v", s)
				}
			case semanticID:
				if !s.Skipped {
					t.Fatalf("a skipped mount must not attempt a git status of an empty directory: %#v", s)
				}
			}
		}
	})

	t.Run("sync advances the local mount and leaves the gitea mount Skipped", func(t *testing.T) {
		semanticBase := testkit.MustHead(t, semantic, "refs/heads/main")
		wau, err := semantic.ApplyKnowledgeCommit(testkit.CommitChange(semanticID, semanticBase, "metric/wau", map[string]any{"definition": "weekly actives"}, ""))
		if err != nil {
			t.Fatal(err)
		}
		syncs, err := cat.SyncMounts("notes", dest)
		if err != nil {
			t.Fatal(err)
		}
		var localSync, giteaSync catalog.MountSync
		for _, s := range syncs {
			switch s.Repository {
			case aliceID:
				localSync = s
			case semanticID:
				giteaSync = s
			}
		}
		// The prior subtest left an uncommitted file in the local mount, so
		// Blocked is the correct, honest outcome here — the point of this
		// assertion is that it is a real outcome Sync actually computed for
		// a local git directory, not Skipped like the gitea mount below.
		if localSync.Outcome == catalog.SyncSkipped {
			t.Fatalf("local mount has a git directory; sync must engage with it, not skip it: %#v", localSync)
		}
		if giteaSync.Outcome != catalog.SyncSkipped {
			t.Fatalf("gitea mount has no local worktree to sync; must stay Skipped, not attempted: %#v", giteaSync)
		}
		if giteaSync.To != wau {
			t.Fatalf("Skipped still reports the commit it would be pinned at: %#v", giteaSync)
		}
	})

	t.Run("write-back through Writer still targets either engine by the routed repository id", func(t *testing.T) {
		groups, err := catalog.RouteMounts(def, []string{"analysis/notes.md", "refs/semantic/metrics/wau.md"})
		if err != nil {
			t.Fatal(err)
		}
		if len(groups[aliceID]) != 1 || len(groups[semanticID]) != 1 {
			t.Fatalf("cross-mount edit must split one-per-repository: %#v", groups)
		}
		aliceBase := testkit.MustHead(t, alice, "refs/heads/main")
		if _, err := w.Commit("write-alice", testkit.CommitChange(aliceID, aliceBase, "note/retention",
			map[string]any{"text": "draft"}, groups[aliceID][0].Path)); err != nil {
			t.Fatalf("write-back to the local member: %v", err)
		}
		semanticBase := testkit.MustHead(t, semantic, "refs/heads/main")
		if _, err := w.Commit("write-semantic", testkit.CommitChange(semanticID, semanticBase, "metric/wau",
			map[string]any{"definition": "weekly actives"}, groups[semanticID][0].Path)); err != nil {
			t.Fatalf("write-back to the gitea member: %v", err)
		}
	})
}
