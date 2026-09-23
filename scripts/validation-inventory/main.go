// Command validation-inventory describes declared coverage; it never executes tests.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"kc/httpsurface"
)

type testEntry struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Package string `json:"package"`
	File    string `json:"file"`
	Line    int    `json:"line"`
}

type commandEntry struct {
	Command string `json:"command"`
	Action  string `json:"action"`
	File    string `json:"file"`
	Line    int    `json:"line"`
}

type routeEntry struct {
	Pattern string `json:"pattern"`
	File    string `json:"file"`
	Line    int    `json:"line"`
}

type referenceEntry struct {
	Document     string      `json:"document"`
	Line         int         `json:"line"`
	Test         string      `json:"test"`
	Declarations []testEntry `json:"declarations"`
}

func testDeclarations(fset *token.FileSet, file *ast.File, path string) []testEntry {
	var entries []testEntry
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name == "TestMain" {
			continue
		}
		for prefix, kind := range map[string]string{"Test": "test", "Benchmark": "benchmark", "Fuzz": "fuzz", "Example": "example"} {
			name := fn.Name.Name
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			suffix := strings.TrimPrefix(name, prefix)
			if suffix != "" && unicode.IsLower([]rune(suffix)[0]) {
				continue
			}
			if !testingSignature(file, fn, prefix) {
				continue
			}
			entries = append(entries, testEntry{name, kind, file.Name.Name, path, fset.Position(fn.Pos()).Line})
		}
	}
	return entries
}

func testingSignature(file *ast.File, fn *ast.FuncDecl, prefix string) bool {
	if fn.Type.TypeParams != nil || fn.Type.Results != nil && len(fn.Type.Results.List) != 0 {
		return false
	}
	params := fn.Type.Params.List
	if prefix == "Example" {
		return len(params) == 0
	}
	if len(params) != 1 || len(params[0].Names) > 1 {
		return false
	}
	ptr, ok := params[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	want := map[string]string{"Test": "T", "Benchmark": "B", "Fuzz": "F"}[prefix]
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != "testing" {
			continue
		}
		name := "testing"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." {
			ident, ok := ptr.X.(*ast.Ident)
			return ok && ident.Name == want
		}
		selector, ok := ptr.X.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		qualifier, ok := selector.X.(*ast.Ident)
		return ok && qualifier.Name == name && selector.Sel.Name == want
	}
	return false
}

func routeDeclarations(fset *token.FileSet, file *ast.File, path string) ([]routeEntry, error) {
	entries := []routeEntry{}
	var shapeErr error
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "HandleFunc" {
			return true
		}
		pattern, ok := literal(call.Args[0])
		if !ok {
			shapeErr = fmt.Errorf("dynamic HTTP registration at %s; update inventory reader", fset.Position(call.Pos()))
			return false
		}
		entries = append(entries, routeEntry{pattern, path, fset.Position(call.Pos()).Line})
		return true
	})
	return entries, shapeErr
}

func literal(expr ast.Expr) (string, bool) {
	value, ok := expr.(*ast.BasicLit)
	if !ok || value.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(value.Value)
	return text, err == nil
}

func cliDeclarations(fset *token.FileSet, file *ast.File, path string) ([]commandEntry, error) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != "cliSurface" {
				continue
			}
			if len(value.Values) != 1 {
				return nil, fmt.Errorf("cliSurface must have one literal value")
			}
			mapping, ok := value.Values[0].(*ast.CompositeLit)
			if !ok {
				return nil, fmt.Errorf("cliSurface is no longer a literal map; update inventory reader")
			}
			entries := []commandEntry{}
			for _, expr := range mapping.Elts {
				pair, ok := expr.(*ast.KeyValueExpr)
				if !ok {
					return nil, fmt.Errorf("unsupported cliSurface entry")
				}
				command, ok := literal(pair.Key)
				if !ok {
					return nil, fmt.Errorf("nonliteral CLI command")
				}
				fields, ok := pair.Value.(*ast.CompositeLit)
				if !ok || len(fields.Elts) != 2 {
					return nil, fmt.Errorf("unsupported CLI action for %s", command)
				}
				action, ok := literal(fields.Elts[1])
				if !ok {
					return nil, fmt.Errorf("nonliteral CLI action for %s", command)
				}
				entries = append(entries, commandEntry{command, action, path, fset.Position(pair.Pos()).Line})
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].Command < entries[j].Command })
			return entries, nil
		}
	}
	return nil, fmt.Errorf("cliSurface not found")
}

func main() {
	check := flag.Bool("check", false, "fail on unresolved exact document Test references; print compact counts")
	flag.Parse()
	if err := generate(*check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(check bool) error {
	raw, err := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z").Output()
	if err != nil {
		return err
	}
	files := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	sort.Strings(files)
	fset := token.NewFileSet()
	tests := []testEntry{}
	commands := []commandEntry{}
	routes := []routeEntry{}
	for _, path := range files {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, "_test.go") {
			tests = append(tests, testDeclarations(fset, file, path)...)
			continue
		}
		if path == "cli/surface.go" {
			commands, err = cliDeclarations(fset, file, path)
			if err != nil {
				return err
			}
		}
		if !strings.HasPrefix(path, "cli/") {
			continue
		}
		found, err := routeDeclarations(fset, file, path)
		if err != nil {
			return err
		}
		routes = append(routes, found...)
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Pattern < routes[j].Pattern })
	if len(commands) == 0 || len(routes) == 0 {
		return fmt.Errorf("public surface inventory is empty")
	}
	patterns := httpsurface.Patterns()
	if len(patterns) != len(routes) {
		return fmt.Errorf("production route inventory %d disagrees with HTTP registry %d", len(routes), len(patterns))
	}
	for i, route := range routes {
		if route.Pattern != patterns[i] {
			return fmt.Errorf("production route %q disagrees with HTTP registry %q", route.Pattern, patterns[i])
		}
	}
	view, err := loadSceneView()
	if err != nil {
		return err
	}
	scenes := map[string]any{
		"states":  view["states"],
		"bundles": view["bundles"],
	}
	agentRaw, err := os.ReadFile("dsh-plugin/scripts/agent-scenarios.json")
	if err != nil {
		return err
	}
	var agents map[string]any
	if err := json.Unmarshal(agentRaw, &agents); err != nil {
		return err
	}
	byName := map[string][]testEntry{}
	for _, entry := range tests {
		byName[entry.Name] = append(byName[entry.Name], entry)
	}
	references := []referenceEntry{}
	unresolved := []referenceEntry{}
	pattern := regexp.MustCompile(`\bTest[A-Z][A-Za-z0-9_]*\b`)
	for _, doc := range validationEvidenceDocuments() {
		raw, err := os.ReadFile(doc)
		if err != nil {
			return err
		}
		for number, line := range bytes.Split(raw, []byte("\n")) {
			for _, name := range pattern.FindAllString(string(line), -1) {
				if name == "TestMain" {
					continue
				} // lifecycle hook, not an executable test declaration
				decls := byName[name]
				if decls == nil {
					decls = []testEntry{}
				}
				ref := referenceEntry{doc, number + 1, name, decls}
				references = append(references, ref)
				if len(decls) == 0 {
					unresolved = append(unresolved, ref)
				}
			}
		}
	}
	counts := map[string]int{"goDeclarations": len(tests), "cliCommands": len(commands), "httpRoutes": len(routes)}
	counts["unresolvedDocumentTestReferences"] = len(unresolved)
	counts["sceneStates"] = jsonArrayLen(view["states"])
	counts["sceneBundles"] = jsonArrayLen(view["bundles"])
	if check {
		for _, ref := range unresolved {
			fmt.Fprintf(os.Stderr, "%s:%d: unresolved %s\n", ref.Document, ref.Line, ref.Test)
		}
		if len(unresolved) > 0 {
			return fmt.Errorf("%d exact document Test references do not resolve", len(unresolved))
		}
		fmt.Printf("inventory: %d CLI commands, %d HTTP routes, %d Go declarations; all exact document Test references resolve\n", len(commands), len(routes), len(tests))
		return nil
	}
	root, err := filepath.Abs(".")
	if err != nil {
		return err
	}
	output := map[string]any{"schemaVersion": 1, "kind": "declared-inventory", "root": root,
		"limitations": []string{"Declarations are not execution results.", "Go inventory includes all tracked and non-ignored test sources across build constraints; runtime subtests come only from run events.", "Scene states, bundles, commands and routes have independent denominators; do not add them into one coverage percentage.", "Documentation references resolve exact Test names only; aliases and file-level links still require human review."},
		"counts":      counts, "goDeclarations": tests, "cliCommands": commands, "httpRoutes": routes,
		"scenes": scenes, "agentScenarios": agents, "documentTestReferences": references,
		"unresolvedDocumentTestReferences": unresolved}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func validationEvidenceDocuments() []string {
	return []string{
		"docs/reviewed/test-catalog.md",
		"docs/reviewed/architecture-invariants.md",
		"docs/reviewed/mvp-acceptance.md",
		"docs/reviewed/refactor-acceptance.md",
	}
}

func loadSceneView() (map[string]any, error) {
	cmd := exec.Command("python3", filepath.Join(".data", "scenes", "tree.py"), "--json")
	out, err := cmd.Output()
	if err != nil {
		msg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			if text := strings.TrimSpace(string(ee.Stderr)); text != "" {
				msg = text
			}
		}
		return nil, fmt.Errorf("scene tree: %s", msg)
	}
	var view map[string]any
	if err := json.Unmarshal(out, &view); err != nil {
		return nil, err
	}
	return view, nil
}

func jsonArrayLen(v any) int {
	switch items := v.(type) {
	case []any:
		return len(items)
	default:
		return 0
	}
}
