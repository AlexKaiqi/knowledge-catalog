package dolt_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	knowledgedolt "kc/knowledge/dolt"
	knowledgemaintenance "kc/knowledge/maintenance"
	"kc/snapshot"
)

func requireRuntime(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("native Dolt adapter test is outside the short suite")
	}
	if _, err := exec.LookPath("dolt"); err == nil {
		return
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("live native Knowledge Dolt adapter is required: neither dolt nor docker is available")
		}
		t.Skip("neither dolt nor docker is available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, docker, "info", "--format", "{{.ServerVersion}}").Run(); err != nil {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatalf("live native Knowledge Dolt adapter is required but Docker daemon is unavailable: %v", err)
		}
		t.Skipf("Docker daemon is unavailable: %v", err)
	}
}

// The scale authority is a native layer-② provider, so it must execute the
// same public Repository and Writer observations as the file-backed providers.
// This is a permanent contract call point, not a one-off probe.
func TestNativeKnowledgeDoltRepositoryAndWriterContracts(t *testing.T) {
	requireRuntime(t)
	factory := func(t *testing.T, id string) snapshot.Store {
		t.Helper()
		repo, err := knowledgedolt.Open(testkit.TempDir(t), kernel.RepositoryID(id))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := repo.Close(); err != nil {
				t.Errorf("close native Dolt repository: %v", err)
			}
		})
		return repo
	}
	testkit.RepositoryContract(t, factory)
	testkit.WriterContract(t, factory)
}

func TestNativeKnowledgeDoltMatchesTreeProviderByOperationStep(t *testing.T) {
	requireRuntime(t)
	treeFactory := func(t *testing.T, id string) snapshot.Store {
		return testkit.MakeTreeStore(t, id)
	}
	nativeFactory := func(t *testing.T, id string) snapshot.Store {
		repo, err := knowledgedolt.Open(testkit.TempDir(t), kernel.RepositoryID(id))
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}
	testkit.ProviderParityContract(t, treeFactory, nativeFactory)
}

func TestNativeKnowledgeDoltRejectsRawTreeWriteCapability(t *testing.T) {
	var repo any = (*knowledgedolt.Repository)(nil)
	if _, ok := repo.(snapshot.TreeStore); ok {
		t.Fatal("native Knowledge authority must not expose raw path writes")
	}
	if _, ok := repo.(knowledge.ChangeStore); !ok {
		t.Fatal("native Knowledge authority lost its typed ChangeSet write capability")
	}
}

func TestNativeKnowledgeRowsReadPageDiffAndTombstone(t *testing.T) {
	requireRuntime(t)
	repo, err := knowledgedolt.Open(t.TempDir(), "kr://native/test")
	if err != nil {
		t.Fatal(err)
	}
	root, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	first, err := repo.ApplyKnowledgeChange("native-1", knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root, Message: "seed",
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:a", AspectName: "structure"}, Value: map[string]any{"name": "a"}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "Table:b"}, Value: map[string]any{"name": "b"}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "Relation:contains"}, Value: map[string]any{
				"relationId": "Relation:contains", "relationType": "contains", "direction": "DIRECTED",
				"endpoints": []any{
					map[string]any{"role": "container", "objectRef": map[string]any{"repository": string(repo.ID()), "object": "Table:a"}},
					map[string]any{"role": "member", "objectRef": map[string]any{"repository": string(repo.ID()), "object": "Table:b"}},
				},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read("Table:a", first)
	if err != nil || value.Value.(map[string]any)["structure"] == nil {
		t.Fatalf("read = %#v, %v", value, err)
	}
	page1, err := repo.ScanSnapshotPage(first, knowledgemaintenance.ScanRequest{Limit: 2})
	if err != nil || len(page1.Values) != 2 || page1.Continuation == "" || page1.Exhausted {
		t.Fatalf("page1 = %#v, %v", page1, err)
	}
	page2, err := repo.ScanSnapshotPage(first, knowledgemaintenance.ScanRequest{Limit: 2, Continuation: page1.Continuation})
	if err != nil || len(page2.Values) != 1 || !page2.Exhausted {
		t.Fatalf("page2 = %#v, %v", page2, err)
	}
	// Relation discovery is intentionally absent from the authority. The full
	// relation remains readable at its fixed canonical commit.
	relationValue, err := repo.Read("Relation:contains", first)
	if err != nil || len(relationValue.Declarations) != 1 || relationValue.Declarations[0].Address.Kind != knowledge.KindRelation {
		t.Fatalf("canonical relation = %#v, %v", relationValue, err)
	}
	if relation, decodeErr := knowledge.DecodeRelation(relationValue.Declarations[0].Address, relationValue.Value); decodeErr != nil || relation.RelationID != "Relation:contains" {
		t.Fatalf("decoded canonical relation = %#v, %v", relation, decodeErr)
	}
	second, err := repo.ApplyKnowledgeChange("native-2", knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: first, ExpectedTargetCommit: first, Message: "remove",
		Operations: []knowledge.Operation{{
			Op: knowledge.OpRemove, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "Table:a"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := repo.Resolve("Table:a", second)
	if err != nil || resolution.Status != knowledge.StatusRemoved {
		t.Fatalf("resolution = %#v, %v", resolution, err)
	}
	diff, err := repo.Diff("Table:a", first, second)
	if err != nil || diff.From == nil || diff.To != nil {
		t.Fatalf("diff = %#v, %v", diff, err)
	}
	changed, err := repo.FastChangedObjectIDs(first, second)
	if err != nil || len(changed) != 1 || changed[0] != "Table:a" {
		t.Fatalf("changed = %#v, %v", changed, err)
	}
	log, err := repo.Log("Table:a", second, knowledge.ObjectLogQuery{Limit: 10})
	if err != nil || len(log) < 2 || log[0].Commit != second {
		t.Fatalf("log = %#v, %v", log, err)
	}
	firstPage, err := repo.Log("Table:a", second, knowledge.ObjectLogQuery{Limit: 1})
	if err != nil || len(firstPage) != 1 || firstPage[0].Commit != second {
		t.Fatalf("first log page = %#v, %v", firstPage, err)
	}
	nextPage, err := repo.Log("Table:a", second, knowledge.ObjectLogQuery{Limit: 1, After: firstPage[0].Commit})
	if err != nil || len(nextPage) == 0 || nextPage[0].Commit == second {
		t.Fatalf("After must be exclusive: %#v, %v", nextPage, err)
	}
	zero, err := repo.Log("Table:a", second, knowledge.ObjectLogQuery{Limit: 0})
	if err != nil || len(zero) != len(log) {
		t.Fatalf("limit 0 must mean the default page: %#v vs %#v, %v", zero, log, err)
	}
}

// The native reverse schema_ref index must answer at a fixed basis, must not
// include the schema object itself, and must stay correct across commits.
func TestNativeSchemaReferrerIndexIsBoundedAndBasisFixed(t *testing.T) {
	requireRuntime(t)
	repo, err := knowledgedolt.Open(t.TempDir(), "kr://native/referrers")
	if err != nil {
		t.Fatal(err)
	}
	root, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	schemaValue := map[string]any{
		"metaSchema": string(knowledge.MetaSchemaV1),
		"entity":     "Table", "aspect": "structure", "pattern": "record",
		"fields": map[string]any{"name": map[string]any{"type": "string", "required": true}},
	}
	first, err := repo.ApplyKnowledgeChange("referrers-1", knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root, Message: "seed",
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/table/structure/v1"}, Value: schemaValue},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:a", AspectName: "structure"},
				SchemaRef: "schema/table/structure/v1", Value: map[string]any{"name": "a"}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:b", AspectName: "structure"},
				SchemaRef: "schema/table/structure/v1", Value: map[string]any{"name": "b"}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "Table:c"}, Value: map[string]any{"name": "c"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	referrers, err := repo.SchemaReferrerAddresses("schema/table/structure/v1", first)
	if err != nil {
		t.Fatal(err)
	}
	if len(referrers) != 2 {
		t.Fatalf("referrers = %#v, want the two declaring units only", referrers)
	}
	for _, address := range referrers {
		if address.AspectName != "structure" || knowledge.IsSchemaObject(address.ObjectID) {
			t.Fatalf("unexpected referrer address %#v", address)
		}
	}
	if none, err := repo.SchemaReferrerAddresses("schema/table/absent/v1", first); err != nil || len(none) != 0 {
		t.Fatalf("unreferenced schema = %#v, %v", none, err)
	}

	second, err := repo.ApplyKnowledgeChange("referrers-2", knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: first, ExpectedTargetCommit: first, Message: "drop one referrer",
		Operations: []knowledge.Operation{{
			Op: knowledge.OpRemove, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:b", AspectName: "structure"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := repo.SchemaReferrerAddresses("schema/table/structure/v1", second)
	if err != nil || len(after) != 1 || after[0].ObjectID != "Table:a" {
		t.Fatalf("referrers after removal = %#v, %v", after, err)
	}
	// The older basis still answers with its own referrers.
	before, err := repo.SchemaReferrerAddresses("schema/table/structure/v1", first)
	if err != nil || len(before) != 2 {
		t.Fatalf("historical referrers = %#v, %v", before, err)
	}
}
