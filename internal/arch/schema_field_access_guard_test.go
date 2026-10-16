package arch_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Retrieval capability lives only in index declaration members. A Schema in
// the system release or in scene materials that still carries field access
// would let a fixture turn green through the retired path (S-01,
// accept-retire-repository-scoped-search).
func TestPublishedAndSceneSchemasCarryNoFieldAccess(t *testing.T) {
	root := moduleRoot(t)
	checked := 0
	for _, dir := range append(publishedSchemaRoots(), ".data/scenes") {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if entry.IsDir() {
				// Counterexample variants deliberately keep rejected shapes.
				if entry.Name() == "variants" || strings.HasPrefix(entry.Name(), "__") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".yaml") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			signals, schema := schemaFieldAccessSignals(string(body))
			if schema {
				checked++
			}
			for _, signal := range signals {
				t.Errorf("%s: %s", rel, signal)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked == 0 {
		t.Fatal("no Schema was checked; the guard lost its denominator")
	}
}

func TestSchemaFieldAccessScanFindsNestedDeclarations(t *testing.T) {
	good := "---\nobject_id: schema/policy\n---\nentity: Policy\nfields:\n  access:\n    type: string\n"
	if signals, schema := schemaFieldAccessSignals(good); !schema || len(signals) != 0 {
		t.Fatalf("a field named access is structure: schema=%v %v", schema, signals)
	}
	bad := "---\nobject_id: schema/table\n---\nentity: Table\nfields:\n  name:\n    type: string\n    access: [text]\n  columns:\n    type: array\n    items:\n      fields:\n        note:\n          type: string\n          access: [filter]\n"
	signals, _ := schemaFieldAccessSignals(bad)
	if !slices.Equal(signals, []string{"field columns.note declares access", "field name declares access"}) {
		t.Fatalf("nested field access not found: %v", signals)
	}
	if _, schema := schemaFieldAccessSignals("---\nobject_id: table/orders\n---\naccess: [x]\n"); schema {
		t.Fatal("non-Schema object counted as Schema")
	}
}

func schemaFieldAccessSignals(body string) ([]string, bool) {
	rest, ok := strings.CutPrefix(body, "---\n")
	if !ok {
		return nil, false
	}
	front, value, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return nil, false
	}
	var header map[string]any
	if yaml.Unmarshal([]byte(front), &header) != nil {
		return nil, false
	}
	id, _ := header["object_id"].(string)
	if !strings.HasPrefix(id, "schema/") {
		return nil, false
	}
	var definition map[string]any
	if yaml.Unmarshal([]byte(value), &definition) != nil {
		return nil, true
	}
	var signals []string
	collectFieldAccess(definition, "", &signals)
	slices.Sort(signals)
	return signals, true
}

func collectFieldAccess(node map[string]any, prefix string, signals *[]string) {
	fields, _ := node["fields"].(map[string]any)
	for name, raw := range fields {
		field, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		path := prefix + name
		if _, declared := field["access"]; declared {
			*signals = append(*signals, "field "+path+" declares access")
		}
		collectFieldAccess(field, path+".", signals)
		if items, ok := field["items"].(map[string]any); ok {
			collectFieldAccess(items, path+".", signals)
		}
	}
}
