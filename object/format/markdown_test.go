package format_test

import (
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
)

func TestMarkdownAspectRoundTripPreservesBody(t *testing.T) {
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "guide/intro", AspectName: "definition"}
	// A colon and a fenced YAML example must remain Markdown, not be decoded as
	// a YAML value. The schema is deliberately not the reserved README schema.
	value := map[string]any{"body": "# Definition\n\nMeaning: object\n\n```yaml\nfield: value\n```"}
	raw, err := repofile.Serialize(address, "guides/intro/definition.md", "schema/guide.definition", nil, value)
	if err != nil {
		t.Fatal(err)
	}
	unit := repofile.Parse(raw)
	if unit == nil || unit.Address != address || unit.SchemaRef != "schema/guide.definition" || kernel.CanonicalDigest(unit.Value) != kernel.CanonicalDigest(value) {
		t.Fatalf("Markdown Aspect failed to round trip: %#v", unit)
	}
}

func TestMarkdownPathKeepsStructuredCanonicalValues(t *testing.T) {
	address := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/plain"}
	for _, value := range []any{"path-derived draft", map[string]any{"text": "structured note"}, map[string]any{"body": "note", "owner": "team"}} {
		raw, err := repofile.Serialize(address, "plain.md", "", nil, value)
		if err != nil {
			t.Fatal(err)
		}
		unit := repofile.Parse(raw)
		if unit == nil || kernel.CanonicalDigest(unit.Value) != kernel.CanonicalDigest(value) {
			t.Fatalf("a Markdown path reinterpreted a Canonical value: %#v", unit)
		}
	}
}
