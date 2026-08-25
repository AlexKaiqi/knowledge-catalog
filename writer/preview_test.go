package writer_test

import (
	"os"
	"path/filepath"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/writer"
)

func TestT7Ingest(t *testing.T) {
	dir := testkit.TempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "policy.md"), []byte("# Policy\nproduction requires a runbook"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes", "oncall.txt"), []byte("check freeze window"), 0o644); err != nil {
		t.Fatal(err)
	}
	preview, err := writer.Ingest(dir, "kr://acme/public/core", "P0")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Files) != 2 || len(preview.ChangeSet.Operations) != 2 {
		t.Fatalf("%#v", preview)
	}
	if preview.ChangeSet.Provenance == nil || preview.ChangeSet.Provenance.OriginKind != knowledge.OriginSource {
		t.Fatal(preview.ChangeSet.Provenance)
	}
	if preview.ChangeSet.Operations[0].Op != knowledge.OpPut {
		t.Fatal(preview.ChangeSet.Operations[0])
	}
}

func TestIngestFrontmatterObjectID(t *testing.T) {
	dir := testkit.TempDir(t)
	nested := filepath.Join(dir, "notes")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nobject_id: policy/P-103\naspect_name: structure\nschema_ref: schema/table@c1\n---\n{\n  \"pk\": [\"id\"]\n}\n"
	if err := os.WriteFile(filepath.Join(nested, "whatever.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	preview, err := writer.Ingest(dir, "kr://acme/public/core", "P0")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Files) != 1 || preview.Files[0].ObjectID != "policy/P-103" {
		t.Fatalf("%#v", preview.Files)
	}
	op := preview.ChangeSet.Operations[0]
	if op.Address.Kind != knowledge.KindAspect || op.Address.AspectName != "structure" || op.SchemaRef != "schema/table@c1" {
		t.Fatalf("%#v", op)
	}
}

func TestT7Reconcile(t *testing.T) {
	snapshot := map[knowledge.ObjectID]any{
		"a": map[string]any{"v": 1},
		"b": map[string]any{"v": 2},
		"c": map[string]any{"v": 3},
	}
	current := map[knowledge.ObjectID]string{
		"a": string(kernel.CanonicalDigest(map[string]any{"v": 1})),
		"b": "stale-digest",
		"d": "will-be-removed",
	}
	preview := writer.Reconcile(snapshot, current, "kr://acme/public/core", "P0")
	if preview.Summary.Added != 1 || preview.Summary.Updated != 1 || preview.Summary.Removed != 1 {
		t.Fatalf("%#v", preview.Summary)
	}
}
