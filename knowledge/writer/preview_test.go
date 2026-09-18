package writer_test

import (
	"os"
	"path/filepath"
	"strings"
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

func TestIngestMarkdownReadmeFrontmatter(t *testing.T) {
	dir := testkit.TempDir(t)
	body := "---\nentity: payments\naspect: readme\nschema_ref: schema/core/readme/v1\n---\n# Payments warehouse\n\nPublished metrics.\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	preview, err := writer.Ingest(dir, "kr://acme/payments", "P0")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.ChangeSet.Operations) != 1 {
		t.Fatalf("%#v", preview.ChangeSet.Operations)
	}
	op := preview.ChangeSet.Operations[0]
	if op.Address.Kind != knowledge.KindAspect || op.Address.ObjectID != "payments" || op.Address.AspectName != knowledge.ReadmeAspect {
		t.Fatalf("%#v", op.Address)
	}
	if op.PathHint != knowledge.RepositoryReadmePath {
		t.Fatalf("path hint %#v", op.PathHint)
	}
	got, ok := knowledge.ReadmeBody(op.Value)
	if !ok || !strings.Contains(got, "Payments warehouse") {
		t.Fatalf("value %#v", op.Value)
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
	if preview.ChangeSet.Operations[0].PathHint != "_schemas/metric.definition.aspect.yaml" {
		t.Fatalf("schema ingest must land under _schemas/: %#v", preview.ChangeSet.Operations[0].PathHint)
	}
}

func TestIngestSchemaOriginFrontmatter(t *testing.T) {
	dir := testkit.TempDir(t)
	body := "---\nobject_id: schema/table.stats\norigin: http://127.0.0.1:7390\n---\nentity: Table\naspect: stats\npattern: record\nfields:\n  rowCount:\n    type: number\n    access: [filter]\n"
	if err := os.WriteFile(filepath.Join(dir, "table.stats.aspect.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	preview, err := writer.Ingest(dir, "kr://dw/physical", "P0")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.ChangeSet.Operations) != 1 {
		t.Fatalf("%#v", preview.ChangeSet.Operations)
	}
	value, ok := preview.ChangeSet.Operations[0].Value.(map[string]any)
	if !ok || value["origin"] != "http://127.0.0.1:7390" || value["aspect"] != "stats" {
		t.Fatalf("frontmatter origin must assemble onto the Schema value: %#v", preview.ChangeSet.Operations[0].Value)
	}
	if preview.ChangeSet.Operations[0].PathHint != "_schemas/table.stats.aspect.yaml" {
		t.Fatalf("schema ingest must land under _schemas/: %#v", preview.ChangeSet.Operations[0].PathHint)
	}
}

func TestIngestRejectsInstanceBinding(t *testing.T) {
	dir := testkit.TempDir(t)
	body := "---\nobject_id: Table:orders\naspect_name: profile\nschema_ref: schema/table.profile\nvalue_source: {\"kind\":\"binding\",\"binding\":{\"mode\":\"state\",\"descriptorRef\":\"resource/mysql\"}}\n---\nnull\n"
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := writer.Ingest(dir, "kr://dw/physical", "P0")
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("instance Binding must be rejected: %v", err)
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

func TestOmitUnchangedKeepsAddsAndUpdates(t *testing.T) {
	added := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "note/new"}
	same := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "note/same"}
	changed := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "note/changed"}
	sameValue := map[string]any{"text": "ok"}
	cs := knowledge.CommitChangeSet{
		TargetRepository: "kr://acme/core",
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: added, Value: map[string]any{"text": "n"}},
			{Op: knowledge.OpPut, Address: same, Value: sameValue},
			{Op: knowledge.OpPut, Address: changed, Value: map[string]any{"text": "next"}},
		},
	}
	filtered, summary := writer.OmitUnchanged(cs, map[string]string{
		knowledge.AddressKey(same):    string(kernel.CanonicalDigest(sameValue)),
		knowledge.AddressKey(changed): "stale",
	})
	if summary.Added != 1 || summary.Updated != 1 || summary.Unchanged != 1 || len(filtered.Operations) != 2 {
		t.Fatalf("%#v %#v", summary, filtered.Operations)
	}
}
