package knowledge_test

import (
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

func TestParseResourceAccessOrigin(t *testing.T) {
	got, err := knowledge.ParseResourceAccessOrigin(" https://stats.example/base/ ")
	if err != nil || got != "https://stats.example/base" {
		t.Fatalf("got %q %v", got, err)
	}
	if got, err := knowledge.ParseResourceAccessOrigin(""); err != nil || got != "" {
		t.Fatalf("empty origin: %q %v", got, err)
	}
	for _, raw := range []string{"file:///tmp/runtime", "https://user:secret@example.com", "https://runtime.example/v1/access", "https://runtime.example?token=x"} {
		if _, err := knowledge.ParseResourceAccessOrigin(raw); err == nil {
			t.Fatalf("accepted invalid origin %q", raw)
		}
	}
}

func TestResourceAccessEndpointRequiresOrigin(t *testing.T) {
	got, err := knowledge.ResourceAccessEndpoint("http://stats:7390")
	if err != nil || got != "http://stats:7390/v1/access" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := knowledge.ResourceAccessEndpoint(""); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("empty origin: %v", err)
	}
}

func TestParseSchemaDefinitionAcceptsOrigin(t *testing.T) {
	definition, err := knowledge.ParseSchemaDefinition("schema/table.stats", map[string]any{
		"entity": "Table", "aspect": "stats", "origin": "https://stats.example",
		"fields": map[string]any{"rowCount": map[string]any{"type": "number", "access": []any{"filter"}}},
	})
	if err != nil || definition.Origin != "https://stats.example" {
		t.Fatalf("%#v %v", definition, err)
	}
	if _, err := knowledge.ParseSchemaDefinition("schema/table.stats", map[string]any{
		"entity": "Table", "aspect": "stats", "origin": "https://stats.example/v1/access",
	}); kernel.CodeOf(err) != kernel.ErrSchemaUnsupported {
		t.Fatalf("invalid origin must be SCHEMA_UNSUPPORTED: %v", err)
	}
	if _, err := knowledge.ParseSchemaDefinition("schema/table.stats", map[string]any{
		"entity": "Table", "origin": "https://stats.example",
	}); kernel.CodeOf(err) != kernel.ErrSchemaUnsupported {
		t.Fatalf("origin without aspect must be SCHEMA_UNSUPPORTED: %v", err)
	}
	if !definition.Bound() || definition.BindingSource() == nil {
		t.Fatalf("bound schema: %#v", definition)
	}
}

func TestAttachAndSplitSchemaOrigin(t *testing.T) {
	body := map[string]any{"entity": "Table", "aspect": "stats"}
	attached, err := knowledge.AttachSchemaOrigin(body, "https://stats.example/")
	if err != nil {
		t.Fatal(err)
	}
	origin, stripped, err := knowledge.SplitSchemaOrigin(attached)
	if err != nil || origin != "https://stats.example" {
		t.Fatalf("split %q %v", origin, err)
	}
	if _, ok := stripped.(map[string]any)["origin"]; ok {
		t.Fatalf("origin remained in body: %#v", stripped)
	}
}
