package writer_test

import (
	"os"
	"path/filepath"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/writer"
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
	for _, file := range preview.Files {
		if file.IdentitySource != "path" {
			t.Fatalf("plain ingest identity source: %#v", file)
		}
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
	if preview.Files[0].IdentitySource != "frontmatter" || preview.Files[0].SchemaRef != "schema/table@c1" {
		t.Fatalf("frontmatter diagnostics: %#v", preview.Files[0])
	}
	op := preview.ChangeSet.Operations[0]
	if op.Address.Kind != knowledge.KindAspect || op.Address.AspectName != "structure" || op.SchemaRef != "schema/table@c1" {
		t.Fatalf("%#v", op)
	}
}

func TestIngestFrontmatterYAMLPayload(t *testing.T) {
	dir := testkit.TempDir(t)
	body := "---\nobject_id: schema/metric.definition\n---\nentity: Metric\naspect: definition\npattern: record\nfields:\n  expression:\n    type: string\n    access: [text]\n"
	if err := os.WriteFile(filepath.Join(dir, "metric.definition.aspect.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	preview, err := writer.Ingest(dir, "kr://dw/semantic", "P0")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.ChangeSet.Operations) != 1 {
		t.Fatalf("%#v", preview.ChangeSet.Operations)
	}
	value, ok := preview.ChangeSet.Operations[0].Value.(map[string]any)
	if !ok || value["entity"] != "Metric" || value["aspect"] != "definition" {
		t.Fatalf("YAML payload was not decoded as a structured knowledge value: %#v", preview.ChangeSet.Operations[0].Value)
	}
	if preview.ChangeSet.Operations[0].PathHint != "schemas/metric.definition.aspect.yaml" {
		t.Fatalf("schema ingest must land under schemas/: %#v", preview.ChangeSet.Operations[0].PathHint)
	}
}

func TestIngestCarriesBindingDeclaration(t *testing.T) {
	dir := testkit.TempDir(t)
	body := "---\nobject_id: Table:orders\naspect_name: profile\nschema_ref: schema/table.profile\nvalue_source: {\"kind\":\"binding\",\"binding\":{\"mode\":\"state\",\"descriptorRef\":\"resource/mysql\"}}\n---\nnull\n"
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	preview, err := writer.Ingest(dir, "kr://dw/physical", "P0")
	if err != nil {
		t.Fatal(err)
	}
	source := preview.ChangeSet.Operations[0].ValueSource
	if source == nil || source.Binding == nil || source.Binding.DescriptorRef != "resource/mysql" {
		t.Fatalf("binding declaration was lost during ingest: %#v", source)
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
