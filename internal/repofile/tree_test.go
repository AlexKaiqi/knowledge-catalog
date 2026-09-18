package repofile

import (
	"strings"
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

func TestApplyRejectsPathHintThatSnapshotReaderCannotDiscover(t *testing.T) {
	idx := NewTree()
	op := knowledge.Operation{
		Op:       knowledge.OpPut,
		Address:  knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:payment-api", AspectName: "observed"},
		Value:    map[string]any{"status": "healthy"},
		PathHint: "service",
	}
	err := Apply(idx, op, nil, map[string]string{}, map[string]struct{}{})
	if err == nil || !strings.Contains(err.Error(), "readable knowledge file extension") {
		t.Fatalf("expected unreadable pathHint rejection, got %v", err)
	}

	op.PathHint = "service.json"
	if err := Apply(idx, op, nil, map[string]string{}, map[string]struct{}{}); err != nil {
		t.Fatalf("valid knowledge pathHint was rejected: %v", err)
	}
}

func TestParseMarkdownReadmeFrontmatter(t *testing.T) {
	content := "---\nentity: payments\naspect: readme\nschema_ref: schema/core/readme/v1\npath_hint: README.md\n---\n# Payments warehouse\n\nPublished metrics.\n"
	unit := Parse(content)
	if unit == nil {
		t.Fatal("parse returned nil")
	}
	if unit.Address.Kind != knowledge.KindAspect || unit.Address.ObjectID != "payments" || unit.Address.AspectName != knowledge.ReadmeAspect {
		t.Fatalf("address %#v", unit.Address)
	}
	body, ok := knowledge.ReadmeBody(unit.Value)
	if !ok || body != "# Payments warehouse\n\nPublished metrics." {
		t.Fatalf("body %#v", unit.Value)
	}
	encoded, err := Serialize(unit.Address, knowledge.RepositoryReadmePath, unit.SchemaRef, nil, unit.Value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(encoded, "entity: payments") || !strings.Contains(encoded, "aspect: readme") || strings.Contains(encoded, "object_id:") {
		t.Fatalf("markdown serialize should use entity/aspect keys: %s", encoded)
	}
	if strings.Contains(encoded, `"body"`) {
		t.Fatalf("markdown body must not be JSON: %s", encoded)
	}
}

func TestKnowledgePathAcceptsYAML(t *testing.T) {
	if !KnowledgePath("gmv/definition.yaml") {
		t.Fatal(".yaml must be a readable knowledge file extension")
	}
	if !KnowledgePath("gmv/definition.okf") {
		t.Fatal("legacy .okf must remain a readable knowledge file extension")
	}
}

func TestSchemaOriginRoundtripsInFrontmatter(t *testing.T) {
	address := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/table.stats"}
	value := map[string]any{
		"entity": "Table", "aspect": "stats", "origin": "https://stats.example",
		"fields": map[string]any{"rowCount": map[string]any{"type": "number"}},
	}
	encoded, err := Serialize(address, "_schemas/table.stats.aspect.yaml", "", nil, value)
	if err != nil {
		t.Fatal(err)
	}
	frontmatter, body, ok := splitCanonical(encoded)
	if !ok {
		t.Fatalf("encoded %#v", encoded)
	}
	if !strings.Contains(frontmatter, "origin: https://stats.example") {
		t.Fatalf("origin must be Canonical frontmatter: %s", encoded)
	}
	if strings.Contains(body, "origin") {
		t.Fatalf("origin must not remain in the Schema body: %s", encoded)
	}
	unit := Parse(encoded)
	if unit == nil {
		t.Fatal("parse returned nil")
	}
	if err := unit.DeclarationError(); err != nil {
		t.Fatal(err)
	}
	got, ok := unit.Value.(map[string]any)
	if !ok || got["origin"] != "https://stats.example" || got["entity"] != "Table" {
		t.Fatalf("assembled value %#v", unit.Value)
	}

	walk := "---\nobject_id: schema/table.stats\norigin: http://127.0.0.1:7390\n---\nentity: Table\naspect: stats\npattern: record\nfields:\n  rowCount:\n    type: number\n"
	parsed := Parse(walk)
	if parsed == nil {
		t.Fatal("walk-style parse returned nil")
	}
	if err := parsed.DeclarationError(); err != nil {
		t.Fatal(err)
	}
	walkValue, ok := parsed.Value.(map[string]any)
	if !ok || walkValue["origin"] != "http://127.0.0.1:7390" || walkValue["aspect"] != "stats" {
		t.Fatalf("walk-style value %#v", parsed.Value)
	}

	instance := Parse("---\nobject_id: table/orders\naspect_name: stats\norigin: https://stats.example\n---\n{}\n")
	if instance == nil || kernel.CodeOf(instance.DeclarationError()) != kernel.ErrUsageInvalid {
		t.Fatalf("instance origin must fail closed: %#v %v", instance, instance.DeclarationError())
	}
}

func splitCanonical(encoded string) (frontmatter, body string, ok bool) {
	if !strings.HasPrefix(encoded, "---\n") {
		return "", "", false
	}
	rest := encoded[4:]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return "", "", false
	}
	return rest[:idx], rest[idx+5:], true
}

func TestIngestRejectsMalformedOrInvalidValueSource(t *testing.T) {
	contents := []string{
		"---\nobject_id: Service:orders\naspect_name: health\nkind: ASPECT\nvalue_source: {bad-json}\n---\nnull\n",
		"---\nobject_id: Service:orders\naspect_name: health\nkind: ASPECT\nvalue_source: {\"kind\":\"binding\",\"binding\":{\"mode\":\"state\"}}\n---\nnull\n",
	}
	for _, content := range contents {
		idx := NewTree()
		if code := kernel.CodeOf(Ingest(idx, Parse(content), "objects/orders/health.json")); code != kernel.ErrUsageInvalid {
			t.Fatalf("invalid declaration must fail closed, got %s", code)
		}
	}
}
