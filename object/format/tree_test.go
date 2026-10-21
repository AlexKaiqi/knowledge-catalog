package format

import (
	"strings"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestApplyRejectsPathHintThatSnapshotReaderCannotDiscover(t *testing.T) {
	idx := NewTree()
	op := objectmodel.Operation{
		Op:       objectmodel.OpPut,
		Address:  objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/payment-api", AspectName: "observed"},
		Value:    map[string]any{"status": "healthy"},
		PathHint: "service",
	}
	err := Apply(idx, op, nil, map[string]string{}, map[string]struct{}{})
	if err == nil || !strings.Contains(err.Error(), "readable object file extension") {
		t.Fatalf("expected unreadable pathHint rejection, got %v", err)
	}

	op.PathHint = "service.json"
	if err := Apply(idx, op, nil, map[string]string{}, map[string]struct{}{}); err != nil {
		t.Fatalf("valid object pathHint was rejected: %v", err)
	}
}

func TestParseMarkdownReadmeFrontmatter(t *testing.T) {
	content := "---\nentity: payments\naspect: readme\nschema_ref: schema/core/readme/v1\npath_hint: README.md\n---\n# Payments warehouse\n\nPublished metrics.\n"
	unit := Parse(content)
	if unit == nil {
		t.Fatal("parse returned nil")
	}
	if unit.Address.Kind != objectmodel.KindAspect || unit.Address.ObjectID != "payments" || unit.Address.AspectName != "readme" {
		t.Fatalf("address %#v", unit.Address)
	}
	body, _ := unit.Value.(map[string]any)["body"].(string)
	if body != "# Payments warehouse\n\nPublished metrics." {
		t.Fatalf("body %#v", unit.Value)
	}
	encoded, err := Serialize(unit.Address, "README.md", unit.SchemaRef, nil, unit.Value)
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

func TestObjectPathAcceptsYAML(t *testing.T) {
	if !ObjectPath("gmv/definition.yaml") {
		t.Fatal(".yaml must be a readable object file extension")
	}
	if !ObjectPath("gmv/definition.okf") {
		t.Fatal("legacy .okf must remain a readable object file extension")
	}
}

func TestBindingUnitRoundtripsAsHeaderOnly(t *testing.T) {
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "stats"}
	source := &objectmodel.ValueSource{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{DescriptorRef: "resource/shop-stats"}}
	encoded, err := SerializeWithSource(address, "tables/orders.stats.yaml", "schema/table.stats", source, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	frontmatter, body, ok := splitCanonical(encoded)
	if !ok || !strings.Contains(frontmatter, `valueSource: {"kind":"binding","binding":{"descriptorRef":"resource/shop-stats"}}`) || strings.TrimSpace(body) != "" {
		t.Fatalf("binding unit must be a header-only unit: %q", encoded)
	}
	unit := Parse(encoded)
	if unit == nil || unit.DeclarationError() != nil || unit.Value != nil || !unit.ValueSource.IsBinding() || unit.ValueSource.Binding.DescriptorRef != "resource/shop-stats" {
		t.Fatalf("parsed %#v", unit)
	}

	walk := "---\nobject_id: table/orders\naspect_name: stats\nschema_ref: schema/table.stats\nvalueSource:\n  kind: binding\n  binding:\n    descriptorRef: resource/shop-stats\n---\n"
	parsed := Parse(walk)
	if parsed == nil || parsed.DeclarationError() != nil || !parsed.ValueSource.IsBinding() || parsed.ValueSource.Binding.DescriptorRef != "resource/shop-stats" {
		t.Fatalf("YAML block valueSource %#v", parsed)
	}
}

func TestRetiredAccessHeadersFailClosed(t *testing.T) {
	for _, content := range []string{
		"---\nobject_id: schema/table.stats\norigin: http://127.0.0.1:7390\n---\nentity: Table\naspect: stats\npattern: record\n",
		"---\nobject_id: table/orders\naspect_name: stats\norigin: https://stats.example\n---\n{}\n",
		"---\nobject_id: table/orders\naspect_name: stats\nvalue_source: {\"kind\":\"binding\",\"binding\":{\"descriptorRef\":\"resource/r\"}}\n---\n",
		"---\nobject_id: table/orders\naspect_name: stats\nvalueSource: {\"kind\":\"binding\",\"binding\":{\"mode\":\"state\"}}\n---\n",
	} {
		unit := Parse(content)
		if unit == nil || kernel.CodeOf(unit.DeclarationError()) != kernel.ErrUsageInvalid {
			t.Fatalf("retired header must fail closed: %q", content)
		}
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
		"---\nobject_id: service/orders\naspect_name: health\nkind: ASPECT\nvalue_source: {bad-json}\n---\nnull\n",
		"---\nobject_id: service/orders\naspect_name: health\nkind: ASPECT\nvalue_source: {\"kind\":\"binding\",\"binding\":{\"mode\":\"state\"}}\n---\nnull\n",
	}
	for _, content := range contents {
		idx := NewTree()
		if code := kernel.CodeOf(Ingest(idx, Parse(content), "objects/orders/health.json")); code != kernel.ErrUsageInvalid {
			t.Fatalf("invalid declaration must fail closed, got %s", code)
		}
	}
}
