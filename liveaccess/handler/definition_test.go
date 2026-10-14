package handler_test

import (
	"testing"

	"kc/kernel"

	interfaces "kc/liveaccess/handler"
)

func TestInterfaceContractValidationAndImplementationPreservation(t *testing.T) {
	for _, body := range []any{nil, map[string]any{"name": "Test"}, map[string]any{"name": "Test", "meaning": "Test", "operations": map[string]any{}}, map[string]any{"name": "Test", "meaning": "Test", "operations": map[string]any{"run": map[string]any{"effect": "unknown"}}}} {
		if _, err := interfaces.ParseInterfaceDefinition("interface/test/v1", body); kernel.CodeOf(err) != kernel.ErrSchemaInstanceInvalid {
			t.Fatalf("invalid contract accepted: %#v %v", body, err)
		}
	}
	body := map[string]any{"interfaceRef": "interface/test/v1", "custom": map[string]any{"domain": "value"}}
	parsed, err := interfaces.ParseInterfaceImplementation(body)
	if err != nil || parsed.InterfaceRef != "interface/test/v1" || parsed.Value["custom"].(map[string]any)["domain"] != "value" {
		t.Fatalf("implementation lost domain declaration: %#v %v", parsed, err)
	}
	definition, err := interfaces.ParseInterfaceDefinition("interface/test/v1", map[string]any{"definition": map[string]any{"name": "Test", "meaning": "Independent contract", "operations": map[string]any{"read": map[string]any{"effect": "read"}}}})
	if err != nil || definition.Digest == "" || definition.Operations["read"].Effect != "read" {
		t.Fatalf("aspect contract: %#v %v", definition, err)
	}
}
