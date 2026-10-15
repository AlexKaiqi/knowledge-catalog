package object_test

import (
	"testing"

	"kc/internal/bindingfixture"
	"kc/kernel"
	objectmodel "kc/object"
)

func TestParseResourceAccessOrigin(t *testing.T) {
	got, err := objectmodel.ParseResourceAccessOrigin(" https://stats.example/base/ ")
	if err != nil || got != "https://stats.example/base" {
		t.Fatalf("got %q %v", got, err)
	}
	if got, err := objectmodel.ParseResourceAccessOrigin(""); err != nil || got != "" {
		t.Fatalf("empty origin: %q %v", got, err)
	}
	for _, raw := range []string{"file:///tmp/runtime", "https://user:secret@example.com", "https://runtime.example/v1/access", "https://runtime.example?token=x"} {
		if _, err := objectmodel.ParseResourceAccessOrigin(raw); err == nil {
			t.Fatalf("accepted invalid origin %q", raw)
		}
	}
}

func TestResourceAccessEndpointRequiresOrigin(t *testing.T) {
	got, err := objectmodel.ResourceAccessEndpoint("http://stats:7390")
	if err != nil || got != "http://stats:7390/v1/access" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := objectmodel.ResourceAccessEndpoint(""); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("empty origin: %v", err)
	}
}

func TestParseSchemaDefinitionRejectsOrigin(t *testing.T) {
	for _, body := range []map[string]any{
		{"entity": "Table", "aspect": "stats", "origin": "https://stats.example", "fields": map[string]any{"rowCount": map[string]any{"type": "number"}}},
		{"entity": "Table", "aspect": "stats", "origin": "https://stats.example/v1/access"},
		{"entity": "Table", "origin": "https://stats.example"},
	} {
		if _, err := objectmodel.ParseSchemaDefinition("schema/table.stats", body); kernel.CodeOf(err) != kernel.ErrSchemaUnsupported {
			t.Fatalf("schema origin must be SCHEMA_UNSUPPORTED: %#v %v", body, err)
		}
	}
	if _, err := objectmodel.ParseSchemaDefinition("schema/table.stats", map[string]any{
		"entity": "Table", "aspect": "stats", "fields": map[string]any{"rowCount": map[string]any{"type": "number", "access": []any{"filter"}}},
	}); err != nil {
		t.Fatalf("structure-only schema rejected: %v", err)
	}
}

func TestParseResourceDescriptorDeclaresOriginAndMode(t *testing.T) {
	got, err := objectmodel.ParseResourceDescriptor("resource/stats", bindingfixture.Descriptor("https://stats.example/"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Origin != "https://stats.example" || got.Mode != objectmodel.BindingState || got.Runtime != bindingfixture.Runtime ||
		got.Protocol != objectmodel.ProtocolResourceAccessV1 || got.Access["lookup"].Call != "lookup" {
		t.Fatalf("descriptor %#v", got)
	}
	if !objectmodel.IsResourceDescriptorValue(bindingfixture.Descriptor("https://stats.example")) || objectmodel.IsResourceDescriptorValue(map[string]any{"entity": "Table"}) {
		t.Fatal("descriptor kind detection")
	}
	described := bindingfixture.Descriptor("https://stats.example")
	described["name"], described["description"] = "stats", "table statistics"
	described["access"] = map[string]any{"lookup": map[string]any{"call": "lookup", "description": "one row"}}
	if _, err := objectmodel.ParseResourceDescriptor("resource/stats", described); err != nil {
		t.Fatalf("documented descriptor rejected: %v", err)
	}
}

func TestParseResourceDescriptorFailsClosed(t *testing.T) {
	mutate := map[string]func(map[string]any){
		"missing origin":     func(b map[string]any) { delete(b, "origin") },
		"origin with path":   func(b map[string]any) { b["origin"] = "https://stats.example/v1/access" },
		"origin with secret": func(b map[string]any) { b["origin"] = "https://user:secret@stats.example" },
		"missing mode":       func(b map[string]any) { delete(b, "mode") },
		"snapshot mode":      func(b map[string]any) { b["mode"] = "snapshot" },
		"missing runtime":    func(b map[string]any) { delete(b, "runtime") },
		"missing protocol":   func(b map[string]any) { delete(b, "protocol") },
		"missing access":     func(b map[string]any) { delete(b, "access") },
		"empty call":         func(b map[string]any) { b["access"] = map[string]any{"lookup": map[string]any{"call": ""}} },
		"operation extras": func(b map[string]any) {
			b["access"] = map[string]any{"lookup": map[string]any{"call": "lookup", "inputSchema": map[string]any{}}}
		},
		"credential header":   func(b map[string]any) { b["headers"] = map[string]any{"Authorization": "Bearer x"} },
		"nested access scope": func(b map[string]any) { b["scope"] = "shop" },
		"wrong kind":          func(b map[string]any) { b["kind"] = "Table" },
	}
	for name, change := range mutate {
		t.Run(name, func(t *testing.T) {
			body := bindingfixture.Descriptor("https://stats.example")
			change(body)
			if _, err := objectmodel.ParseResourceDescriptor("resource/stats", body); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
				t.Fatalf("must be USAGE_INVALID, got %v", err)
			}
		})
	}
}
