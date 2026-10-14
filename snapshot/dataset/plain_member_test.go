package dataset_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

// plainSnapshot is an ordinary git repo as the Store sees it: embedding the
// Store interface drops optional tree and object methods from the method set, so
// no amount of type assertion recovers layer ②.
type plainSnapshot struct{ snapshot.Store }

// plainTreeSnapshot exposes only the literal tree half of the backing adapter.
// Writer owns the ② codec, so this is writable without becoming an Object
// reader or leaking PUT/REMOVE into snapshot.Store.
type plainTreeSnapshot struct {
	snapshot.Store
	snapshot.TreeStore
}

func mountPlain(t *testing.T, repositoryID string) (snapshot.Registry, plainSnapshot) {
	t.Helper()
	plain := plainSnapshot{Store: testkit.MakeRepository(t, repositoryID)}
	store := snapshot.NewRegistry()
	if err := store.Add(plain); err != nil {
		t.Fatal(err)
	}
	return store, plain
}

// Both codes are raised elsewhere for unrelated reasons, so the wording is part
// of the assertion: it proves the refusal came from the missing capability.
func expectMissingObject(t *testing.T, err error, code kernel.ErrorCode) {
	t.Helper()
	testkit.ExpectCode(t, err, code)
}

// Composition is layer ⓪+①: a repo nobody taught to interpret object files
// still mounts, joins a workspace and resolves to a commit.
func TestPlainGitRepositoryComposesWithoutObjectCapability(t *testing.T) {
	store, plain := mountPlain(t, "kr://acme/personals/alice")
	cat := testkit.OpenDatasets(t, store)
	if _, err := cat.DefineDataset("notes", 1, []catalog.DatasetSource{
		{Repository: plain.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}

	resolved, err := cat.ResolveDataset("notes")
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
func TestObjectCapabilityIsReportedAtTheSeam(t *testing.T) {
	store, plain := mountPlain(t, "kr://acme/personals/alice")
	cat := testkit.OpenDatasets(t, store)
	if _, err := cat.Require(plain.ID()); err != nil {
		t.Fatalf("composition must not require layer ②: %v", err)
	}
	_, err := reader.NewReader(store).Require(plain.ID(), kernel.ErrCapabilityUnsatisfied)
	expectMissingObject(t, err, kernel.ErrCapabilityUnsatisfied)

	if _, err := cat.DefineDataset("notes", 1, []catalog.DatasetSource{
		{Repository: plain.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	_, err = testkit.FederatedRead(cat, "notes", "metric/dau")
	expectMissingObject(t, err, kernel.ErrCapabilityUnsatisfied)
}

// Writing files into a plain repo is layer ⓪ work. Only a claimed schema_ref
// needs the target to resolve schema/*, and then it says so.
func TestCommitToPlainMemberUntilSchemaRefIsClaimed(t *testing.T) {
	backing := testkit.MakeRepository(t, "kr://acme/personals/alice")
	plain := plainTreeSnapshot{Store: backing, TreeStore: backing}
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
	expectMissingObject(t, err, kernel.ErrSchemaRevisionUnresolved)
}
