package knowledge

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSchemaDocumentJSONSchemaMatchesBuiltinContract(t *testing.T) {
	doc := loadSchemaDocumentJSONSchema(t)
	defs, ok := doc["$defs"].(map[string]any)
	if !ok {
		t.Fatal("schema-document.schema.yaml missing $defs")
	}
	gotAccess := enumSet(t, defs, "access")
	for hint := range supportedSchemaAccess {
		if _, ok := gotAccess[hint]; !ok {
			t.Fatalf("JSON Schema access enum missing %s", hint)
		}
		delete(gotAccess, hint)
	}
	if len(gotAccess) != 0 {
		t.Fatalf("JSON Schema access enum extras %v", gotAccess)
	}

	gotTypes := enumSet(t, defs, "fieldType")
	for fieldType := range supportedSchemaTypes {
		if fieldType == "" {
			continue
		}
		if _, ok := gotTypes[fieldType]; !ok {
			t.Fatalf("JSON Schema fieldType missing %s", fieldType)
		}
		delete(gotTypes, fieldType)
	}
	if len(gotTypes) != 0 {
		t.Fatalf("JSON Schema fieldType extras %v", gotTypes)
	}

	pattern, ok := doc["properties"].(map[string]any)["pattern"].(map[string]any)
	if !ok {
		t.Fatal("properties.pattern")
	}
	gotPattern := map[string]struct{}{}
	for _, item := range enumStrings(t, pattern) {
		gotPattern[item] = struct{}{}
	}
	for _, token := range []string{"record", "keyed_collection"} {
		if _, ok := gotPattern[token]; !ok {
			t.Fatalf("JSON Schema pattern missing %s", token)
		}
	}
}

func loadSchemaDocumentJSONSchema(t *testing.T) map[string]any {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "schema-document.schema.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func enumSet(t *testing.T, defs map[string]any, name string) map[string]struct{} {
	t.Helper()
	node, ok := defs[name].(map[string]any)
	if !ok {
		t.Fatalf("$defs.%s", name)
	}
	got := map[string]struct{}{}
	for _, token := range enumStrings(t, node) {
		got[token] = struct{}{}
	}
	return got
}

func enumStrings(t *testing.T, node map[string]any) []string {
	t.Helper()
	raw, ok := node["enum"].([]any)
	if !ok {
		t.Fatalf("missing enum: %#v", node)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok {
			t.Fatalf("enum item %T", item)
		}
		out = append(out, s)
	}
	return out
}
