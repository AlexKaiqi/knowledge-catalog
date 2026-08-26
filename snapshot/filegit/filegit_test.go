package filegit_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"kc/internal/gitdir"
	"kc/internal/repofile"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot/filegit"
)

type knowledgeWriter interface {
	knowledge.Repository
	ApplyKnowledgeCommit(knowledge.ChangeSet) (kernel.CommitID, error)
}

func apply(t *testing.T, repo knowledgeWriter, base kernel.CommitID, ops []knowledge.Operation, message string, prov *knowledge.ProvenanceEnvelope) kernel.CommitID {
	t.Helper()
	cs := knowledge.CommitChangeSet{
		TargetRepository:     repo.ID(),
		TargetRef:            "HEAD",
		BaseCommit:           base,
		ExpectedTargetCommit: base,
		Operations:           ops,
		Message:              message,
		Provenance:           prov,
	}
	commit, err := repo.ApplyKnowledgeCommit(cs)
	if err != nil {
		t.Fatal(err)
	}
	return commit
}

func TestFileGitFrontmatterAndMove(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	root := testkit.MustHead(t, repo, "")
	c1 := apply(t, repo, root, testkit.PutEntity("policy/P-103", map[string]any{"statement": "v1"}, "policies/P-103.json"), "", &knowledge.ProvenanceEnvelope{OriginKind: knowledge.OriginDefinition, ActorRef: "core-council"})
	if c1 == root {
		t.Fatal("commit did not move")
	}
	res, err := repo.Resolve("policy/P-103", c1)
	if err != nil || res.Status != knowledge.StatusResolved || res.PathHint != "policies/P-103.json" {
		t.Fatalf("%#v %v", res, err)
	}
	c2 := apply(t, repo, c1, testkit.PutEntity("policy/P-103", map[string]any{"statement": "v1"}, "policies/production/P-103.json"), "", nil)
	res, err = repo.Resolve("policy/P-103", c2)
	if err != nil || res.PathHint != "policies/production/P-103.json" {
		t.Fatalf("%#v %v", res, err)
	}
}

func TestFileGitStaleCAS(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	root := testkit.MustHead(t, repo, "")
	apply(t, repo, root, testkit.PutEntity("a", 1, ""), "", nil)
	_, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "HEAD",
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: testkit.PutEntity("b", 2, ""),
	})
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
}

func TestFileGitProvenance(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	root := testkit.MustHead(t, repo, "")
	c1 := apply(t, repo, root, testkit.PutEntity("policy/P-103", map[string]any{"statement": "v1"}, "policies/P-103.json"), "", &knowledge.ProvenanceEnvelope{
		OriginKind: knowledge.OriginDefinition, ActorRef: "core-council", SourceRefs: []string{"handbook-v1"},
	})
	trace, err := repo.GetProvenance("policy/P-103", c1)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Chain) != 1 || trace.Chain[0].OriginKind != knowledge.OriginDefinition || trace.Chain[0].ActorRef != "core-council" {
		t.Fatalf("%#v", trace)
	}
}

func TestFileGitPathEscape(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	root := testkit.MustHead(t, repo, "")
	_, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "HEAD",
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: testkit.PutEntity("escape", 1, "../escape.json"),
	})
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

func TestFileGitMergeFastForward(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	if err := repo.CreateRef("refs/heads/candidate", root); err != nil {
		t.Fatal(err)
	}
	candidate, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/candidate",
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: testkit.PutEntity("candidate", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	main, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: testkit.PutEntity("main", 2, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Merge("refs/heads/main", candidate, main)
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
}

func TestFileGitMessageNotShell(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	root := testkit.MustHead(t, repo, "")
	marker := filepath.Join(repo.RootDir(), "PWNED")
	apply(t, repo, root, testkit.PutEntity("safe-message", 1, ""), "message\"; touch "+marker+"; #", nil)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("message was executed as a shell command")
	}
}

func TestFileGitPinnedTreeNotDirty(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	root := testkit.MustHead(t, repo, "")
	commit := apply(t, repo, root, testkit.PutEntity("a", map[string]any{"value": "committed"}, "objects/a.json"), "", nil)
	if err := os.WriteFile(filepath.Join(repo.RootDir(), "objects/a.json"), []byte("---\nobject_id: a\n---\n{\"value\":\"dirty\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read("a", commit)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := value.Value.(map[string]any)
	if got["value"] != "committed" {
		t.Fatalf("%#v", value.Value)
	}
}

func TestFileGitDerivationProvenance(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	root := testkit.MustHead(t, repo, "")
	_, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "HEAD",
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: testkit.PutEntity("derived", 1, ""),
		Provenance: &knowledge.ProvenanceEnvelope{OriginKind: knowledge.OriginDerivation, InputWorkspaceVersionRef: "vr-1"},
	})
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
	commit, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "HEAD",
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: testkit.PutEntity("derived", 1, ""),
		Provenance: &knowledge.ProvenanceEnvelope{
			OriginKind:               knowledge.OriginDerivation,
			InputWorkspaceVersionRef: "vr-1",
			Algorithm:                &knowledge.AlgorithmRef{CodeHash: "sha256:abc"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read("derived", commit)
	if err != nil {
		t.Fatal(err)
	}
	if value.Value != float64(1) && value.Value != 1 {
		t.Fatalf("%#v", value.Value)
	}
}

func TestFileGitAspects(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	root := testkit.MustHead(t, repo, "")
	c1 := apply(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:tl.db.t", AspectName: "structure"}, Value: map[string]any{"storage_type": "hive"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:tl.db.t", AspectName: "ownership"}, Value: map[string]any{"owner": "alice"}},
	}, "", nil)
	value, err := repo.Read("Table:tl.db.t", c1)
	if err != nil {
		t.Fatal(err)
	}
	got := value.Value.(map[string]any)
	if got["structure"].(map[string]any)["storage_type"] != "hive" || got["ownership"].(map[string]any)["owner"] != "alice" {
		t.Fatalf("%#v", value.Value)
	}
	listed, err := repo.List(c1)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list %v %v", listed, err)
	}
	unit, err := repo.ReadAddress(knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:tl.db.t", AspectName: "structure"}, c1)
	if err != nil || unit.Value.(map[string]any)["storage_type"] != "hive" {
		t.Fatalf("%#v %v", unit, err)
	}
}

func TestFileGitPerAspectCAS(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	root := testkit.MustHead(t, repo, "")
	c1 := apply(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "t", AspectName: "structure"}, Value: map[string]any{"pk": []any{"id"}}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "t", AspectName: "ownership"}, Value: map[string]any{"owner": "alice"}},
	}, "", nil)
	structure, err := repo.ResolveAddress(knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "t", AspectName: "structure"}, c1)
	if err != nil {
		t.Fatal(err)
	}
	c2 := apply(t, repo, c1, []knowledge.Operation{{
		Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "t", AspectName: "structure"},
		Value: map[string]any{"pk": []any{"id", "ds"}}, Precondition: &knowledge.Precondition{Type: knowledge.IfDigestEquals, Digest: structure.Digest},
	}}, "", nil)
	value, err := repo.Read("t", c2)
	if err != nil {
		t.Fatal(err)
	}
	got := value.Value.(map[string]any)
	if got["ownership"].(map[string]any)["owner"] != "alice" {
		t.Fatalf("%#v", value.Value)
	}
	_, err = repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "HEAD",
		BaseCommit: c2, ExpectedTargetCommit: c2,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "t", AspectName: "structure"},
			Value: map[string]any{"pk": []any{"x"}}, Precondition: &knowledge.Precondition{Type: knowledge.IfDigestEquals, Digest: structure.Digest},
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
}

func TestFileGitRejectBlobAspectMix(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	root := testkit.MustHead(t, repo, "")
	c1 := apply(t, repo, root, testkit.PutEntity("mixed", map[string]any{"blob": true}, ""), "", nil)
	_, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "HEAD",
		BaseCommit: c1, ExpectedTargetCommit: c1,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "mixed", AspectName: "structure"}, Value: map[string]any{"x": 1},
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrObjectIDConflict)
}

func TestFileGitRejectsDuplicateAddressInExistingGit(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	address := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "duplicate"}
	body, err := repofile.Serialize(address, "", "", nil, map[string]any{"v": 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"one.json", "nested/two.json"} {
		path := filepath.Join(repo.RootDir(), rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := gitdir.At(repo.RootDir())
	if err := git.StageAll(); err != nil {
		t.Fatal(err)
	}
	commit, err := git.Commit(gitdir.Signature{Message: "duplicate address"}, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Read("duplicate", kernel.CommitID(commit))
	testkit.ExpectCode(t, err, kernel.ErrObjectIDConflict)
}

func TestFileGitMembersAndRemoveAspect(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	root := testkit.MustHead(t, repo, "")
	c1 := apply(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "t", AspectName: "structure"}, Value: map[string]any{"ok": true}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindMember, ObjectID: "t", AspectName: "permissions", MemberKey: "user:a"}, Value: map[string]any{"privileges": []any{"SELECT"}}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindMember, ObjectID: "t", AspectName: "permissions", MemberKey: "user:b"}, Value: map[string]any{"privileges": []any{"ALL"}}},
	}, "", nil)
	value, err := repo.Read("t", c1)
	if err != nil {
		t.Fatal(err)
	}
	perms := value.Value.(map[string]any)["permissions"].(map[string]any)
	if perms["user:a"].(map[string]any)["privileges"].([]any)[0] != "SELECT" {
		t.Fatalf("%#v", value.Value)
	}
	c2 := apply(t, repo, c1, []knowledge.Operation{{
		Op: knowledge.OpRemove, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "t", AspectName: "structure"},
	}}, "", nil)
	value, err = repo.Read("t", c2)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := value.Value.(map[string]any)["structure"]; ok {
		t.Fatalf("structure still present: %#v", value.Value)
	}
}

func TestFileGitStamp(t *testing.T) {
	dir := testkit.TempDir(t)
	id := kernel.RepositoryID("kr://acme/public/core")
	repo, err := filegit.NewFileGit(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	got, driver, err := filegit.ReadFileGitStamp(repo.RootDir())
	if err != nil || got != string(id) || driver != "filegit" {
		t.Fatalf("%s %s %v", got, driver, err)
	}
	_, err = filegit.NewFileGit(dir, "kr://acme/other")
	if err == nil || !strings.Contains(err.Error(), "stamped") {
		t.Fatalf("%v", err)
	}
}

func TestAttachGitDoesNotStampOrWriteExclude(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plain")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	id := kernel.RepositoryID("kr://acme/personals/alice")
	repo, err := filegit.AttachGit(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if repo.ID() != id || repo.RootDir() != dir {
		t.Fatalf("%s %s", repo.ID(), repo.RootDir())
	}
	if _, _, err := filegit.ReadFileGitStamp(dir); err == nil {
		t.Fatal("AttachGit must not stamp kc.repositoryId into someone else's git config")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
	if strings.Contains(string(raw), "streams/") {
		t.Fatalf("AttachGit must not write streams/ exclude into an external repo: %s", raw)
	}
}

func TestOpenExistingFileGitIsReadOnlyUnderConcurrentReaders(t *testing.T) {
	dir := testkit.TempDir(t)
	id := kernel.RepositoryID("kr://acme/shared/semantic")
	if _, err := filegit.NewFileGit(dir, id); err != nil {
		t.Fatal(err)
	}
	const readers = 24
	var wg sync.WaitGroup
	errs := make(chan error, readers)
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			repo, err := filegit.NewFileGit(dir, id)
			if err == nil {
				_, err = repo.Head("refs/heads/main")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent open must not contend on .git/config: %v", err)
		}
	}
}
