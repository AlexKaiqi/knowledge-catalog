package arch_test

import (
	"go/ast"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The value-declaration guard keeps one resolution path: a unit header
// valueSource names a ResourceDescriptor, and nothing reads origin or binding
// from a Schema first and falls back to the instance. Field-level access in
// published schemas is owned by the index-declaration guard, not this one.

var schemaDeclarationKeys = []string{"origin", "binding", "valueSource", "value_source", "descriptorRef"}

var topLevelDeclarationKey = regexp.MustCompile(`^(origin|binding|valueSource|value_source|descriptorRef)\s*:`)

func TestValueDeclarationResolvesOnlyFromUnitValueSource(t *testing.T) {
	files := productionSourceFiles(t, nil)
	if signals := valueDeclarationSignals(files); len(signals) != 0 {
		t.Fatalf("value declaration path regressed:\n%s", strings.Join(signals, "\n"))
	}
}

func TestValueDeclarationGuardRejectsSchemaFallbacks(t *testing.T) {
	base := valueDeclarationFixture(t)
	if signals := valueDeclarationSignals(base); len(signals) != 0 {
		t.Fatalf("single valueSource path rejected: %v", signals)
	}
	for _, fixture := range []struct{ name, path, source, want string }{
		{"second-constructor", "liveaccess/schema_binding.go",
			"package liveaccess; func FromSchema() ResolvedBinding { return ResolvedBinding{Mode: \"state\"} }",
			"constructs ResolvedBinding outside ResolveDeclaration"},
		{"schema-lookup-in-path", "liveaccess/binding.go",
			"package liveaccess; import objectmodel \"kc/object\"; func ResolveDeclaration(d objectmodel.UnitDeclaration) ResolvedBinding { s := d.ValueSource; _, _ = objectmodel.ParseSchemaDefinition(\"schema/x\", nil); _ = s; return ResolvedBinding{Mode: \"state\"} }",
			"ResolveDeclaration consults Schema"},
		{"no-value-source", "liveaccess/binding.go",
			"package liveaccess; func ResolveDeclaration() ResolvedBinding { return ResolvedBinding{Mode: \"state\"} }",
			"ResolveDeclaration does not read the unit ValueSource"},
		{"schema-origin-field", "object/schema.go",
			"package object; type SchemaFieldDefinition struct{}; type SchemaDefinition struct{ Origin string `json:\"origin,omitempty\"` }",
			"SchemaDefinition declares origin"},
		{"field-binding", "object/schema.go",
			"package object; type SchemaDefinition struct{}; type SchemaFieldDefinition struct{ Binding any `json:\"binding\"` }",
			"SchemaFieldDefinition declares binding"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			files := map[string]*ast.File{}
			for path, file := range base {
				files[path] = file
			}
			files[fixture.path] = declarativeIndexGuardFixture(t, fixture.path, fixture.source)
			if signals := valueDeclarationSignals(files); !slices.ContainsFunc(signals, func(s string) bool { return strings.Contains(s, fixture.want) }) {
				t.Fatalf("%s was not identified: %v", fixture.want, signals)
			}
		})
	}
	missing := map[string]*ast.File{"object/schema.go": base["object/schema.go"]}
	if signals := valueDeclarationSignals(missing); !slices.ContainsFunc(signals, func(s string) bool { return strings.Contains(s, "one ResolveDeclaration") }) {
		t.Fatalf("missing declaration path was accepted: %v", signals)
	}
}

func TestPublishedSchemasDeclareNoSchemaLevelOrigin(t *testing.T) {
	root := moduleRoot(t)
	checked := 0
	for _, dir := range publishedSchemaRoots() {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !strings.Contains(string(body), "\nobject_id: schema/") && !strings.HasPrefix(string(body), "---\nobject_id: schema/") {
				return nil
			}
			checked++
			rel, _ := filepath.Rel(root, path)
			for _, signal := range schemaLevelDeclarationSignals(string(body)) {
				t.Errorf("%s: %s", filepath.ToSlash(rel), signal)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked == 0 {
		t.Fatal("no published schema was checked; the guard lost its denominator")
	}
}

func TestSchemaLevelDeclarationScanRejectsOriginAndBinding(t *testing.T) {
	good := "---\nobject_id: schema/table.stats\n---\nentity: Table\nfields:\n  origin:\n    type: string\n"
	if signals := schemaLevelDeclarationSignals(good); len(signals) != 0 {
		t.Fatalf("a field named origin is structure, not a declaration: %v", signals)
	}
	for _, key := range schemaDeclarationKeys {
		bad := "---\nobject_id: schema/table.stats\n---\nentity: Table\n" + key + ": https://stats.example\n"
		if signals := schemaLevelDeclarationSignals(bad); len(signals) != 1 {
			t.Fatalf("schema-level %s was not rejected: %v", key, signals)
		}
	}
}

func publishedSchemaRoots() []string {
	return []string{"systemrepo/content", "object/trusted", "liveaccess/contract", ".data/scenes/service-initialized/_materials/system"}
}

func schemaLevelDeclarationSignals(body string) []string {
	var signals []string
	for i, line := range strings.Split(body, "\n") {
		if match := topLevelDeclarationKey.FindStringSubmatch(line); match != nil {
			signals = append(signals, "line "+strconv.Itoa(i+1)+" declares Schema-level "+match[1])
		}
	}
	return signals
}

func valueDeclarationSignals(files map[string]*ast.File) []string {
	var signals []string
	declarations := 0
	for path, file := range files {
		for _, decl := range file.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok {
				signals = append(signals, schemaShapeSignals(path, gen)...)
				continue
			}
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			owner := strings.HasPrefix(path, "liveaccess/") && !strings.Contains(strings.TrimPrefix(path, "liveaccess/"), "/") && fn.Recv == nil && fn.Name.Name == "ResolveDeclaration"
			if owner {
				declarations++
				signals = append(signals, resolveDeclarationSignals(path, fn)...)
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				literal, ok := node.(*ast.CompositeLit)
				if ok && len(literal.Elts) > 0 && typeName(literal.Type) == "ResolvedBinding" && !owner {
					signals = append(signals, path+": "+fn.Name.Name+" constructs ResolvedBinding outside ResolveDeclaration")
				}
				return true
			})
		}
	}
	if declarations != 1 {
		signals = append(signals, "want one ResolveDeclaration in package liveaccess, found "+strconv.Itoa(declarations))
	}
	slices.Sort(signals)
	return signals
}

func resolveDeclarationSignals(path string, fn *ast.FuncDecl) []string {
	var signals []string
	readsValueSource := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SelectorExpr:
			switch n.Sel.Name {
			case "ValueSource":
				readsValueSource = true
			case "ParseSchemaDefinition", "SchemaDefinition", "SchemaFieldDefinition", "SchemaRefs", "BindingSchemaObjectIDs":
				signals = append(signals, path+": ResolveDeclaration consults Schema via "+n.Sel.Name)
			}
		}
		return true
	})
	if !readsValueSource {
		signals = append(signals, path+": ResolveDeclaration does not read the unit ValueSource")
	}
	return signals
}

func schemaShapeSignals(path string, gen *ast.GenDecl) []string {
	if path != "object/schema.go" {
		return nil
	}
	var signals []string
	for _, spec := range gen.Specs {
		typ, ok := spec.(*ast.TypeSpec)
		if !ok || (typ.Name.Name != "SchemaDefinition" && typ.Name.Name != "SchemaFieldDefinition") {
			continue
		}
		structType, ok := typ.Type.(*ast.StructType)
		if !ok {
			continue
		}
		for _, field := range structType.Fields.List {
			if field.Tag == nil {
				continue
			}
			tag, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				continue
			}
			name, _, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ",")
			if slices.Contains(schemaDeclarationKeys, name) {
				signals = append(signals, path+": "+typ.Name.Name+" declares "+name)
			}
		}
	}
	return signals
}

func typeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	}
	return ""
}

func valueDeclarationFixture(t *testing.T) map[string]*ast.File {
	t.Helper()
	return map[string]*ast.File{
		"object/schema.go":      declarativeIndexGuardFixture(t, "schema.go", "package object; type SchemaFieldDefinition struct{ Path string `json:\"path\"`; Access []string `json:\"access,omitempty\"` }; type SchemaDefinition struct{ Entity string `json:\"entity\"` }"),
		"liveaccess/binding.go": declarativeIndexGuardFixture(t, "binding.go", "package liveaccess; import objectmodel \"kc/object\"; func ResolveDeclaration(d objectmodel.UnitDeclaration) ResolvedBinding { s := d.ValueSource; _ = s; return ResolvedBinding{Mode: \"state\"} }"),
		"liveaccess/scope.go":   declarativeIndexGuardFixture(t, "scope.go", "package liveaccess; func All() []ResolvedBinding { return []ResolvedBinding{} }"),
	}
}
