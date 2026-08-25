package catalog_test

import (
	"strings"
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
	"kc/writer"
)

// plainSnapshot is an ordinary git repo as the Store sees it: embedding the
// Store interface drops optional tree and knowledge methods from the method set, so
// no amount of type assertion recovers layer ②.
type plainSnapshot struct{ snapshot.Store }

// plainTreeSnapshot exposes only the literal tree half of the backing adapter.
// Writer owns the ② codec, so this is writable without becoming a Knowledge
// reader or leaking PUT/REMOVE into snapshot.Store.
type plainTreeSnapshot struct {
	snapshot.Store
	snapshot.TreeStore
}

func mountPlain(t *testing.T, repositoryID string) (*snapshot.Registry, plainSnapshot) {
	t.Helper()
	plain := plainSnapshot{Store: testkit.MakeRepository(t, repositoryID)}
	if _, ok := knowledge.Of(plain); ok {
		t.Fatal("fixture must not interpret knowledge files")
	}
	store := snapshot.NewRegistry()
	if err := store.Add(plain); err != nil {
		t.Fatal(err)
	}
	return store, plain
}

// Both codes are raised elsewhere for unrelated reasons, so the wording is part
// of the assertion: it proves the refusal came from the missing capability.
func expectMissingKnowledge(t *testing.T, err error, code kernel.ErrorCode) {
	t.Helper()
	testkit.ExpectCode(t, err, code)
	if !strings.Contains(err.Error(), "plain snapshot") {
		t.Fatalf("refusal must name the missing capability, got %v", err)
	}
}

// Composition is layer ⓪+①: a repo nobody taught to interpret knowledge files
// still mounts, joins a workspace and resolves to a commit.
func TestPlainGitRepositoryComposesWithoutKnowledgeCapability(t *testing.T) {
	store, plain := mountPlain(t, "kr://acme/personals/alice")
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{
		{Repository: plain.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}

	resolved, err := cat.ResolveWorkspace("notes")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Repositories[plain.ID()] == "" {
		t.Fatalf("plain member must resolve to a commit: %#v", resolved.Repositories)
	}
	if check := cat.CheckResolved(resolved); check.Outcome != "PASSED" {
		t.Fatal(check)
	}
}

// The capability is reported where it is needed, not assumed at mount time.
func TestKnowledgeCapabilityIsReportedAtTheSeam(t *testing.T) {
	store, plain := mountPlain(t, "kr://acme/personals/alice")
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.Require(plain.ID()); err != nil {
		t.Fatalf("composition must not require layer ②: %v", err)
	}
	_, err := knowledge.Require(store, plain.ID(), kernel.ErrUsageInvalid)
	expectMissingKnowledge(t, err, kernel.ErrCapabilityUnsatisfied)

	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{
		{Repository: plain.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	_, err = testkit.FederatedRead(cat, "notes", "metric/dau")
	expectMissingKnowledge(t, err, kernel.ErrCapabilityUnsatisfied)
}

// Writing files into a plain repo is layer ⓪ work. Only a claimed schema_ref
// needs the target to resolve schema/*, and then it says so.
func TestCommitToPlainMemberUntilSchemaRefIsClaimed(t *testing.T) {
	backing := testkit.MakeRepository(t, "kr://acme/personals/alice")
	plain := plainTreeSnapshot{Store: backing, TreeStore: backing}
	if _, ok := knowledge.Of(plain); ok {
		t.Fatal("fixture must not interpret knowledge files")
	}
	store := snapshot.NewRegistry()
	if err := store.Add(plain); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	base, err := plain.Head("refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}

	receipt, err := w.Commit("cmd-plain", testkit.CommitChange(plain.ID(), base, "note/churn", map[string]any{"text": "draft"}, ""))
	if err != nil {
		t.Fatalf("plain target must accept a commit that claims no schema: %v", err)
	}
	if receipt.Result.NewCommit == base {
		t.Fatal("commit did not advance the ref")
	}

	cs := testkit.CommitChange(plain.ID(), receipt.Result.NewCommit, "note/retention", map[string]any{"text": "draft"}, "")
	cs.Operations[0].SchemaRef = "kc://acme/personals/alice/schema/note@" + string(receipt.Result.NewCommit)
	_, err = w.Commit("cmd-schema", cs)
	expectMissingKnowledge(t, err, kernel.ErrSchemaRevisionUnresolved)
}
