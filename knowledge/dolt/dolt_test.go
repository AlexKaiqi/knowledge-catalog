package dolt_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

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
		t.Skip("neither dolt nor docker is available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, docker, "info", "--format", "{{.ServerVersion}}").Run(); err != nil {
		t.Skipf("Docker daemon is unavailable: %v", err)
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
	log, err := repo.Log("Table:a", second, 10)
	if err != nil || len(log) < 2 || log[0].Commit != second {
		t.Fatalf("log = %#v, %v", log, err)
	}
}
