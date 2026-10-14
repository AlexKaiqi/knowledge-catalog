package arch_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	snapshotcli "kc/snapshot/cli"
)

// Goal: keep Snapshot leaf registration, operands, dispatch and output with
// their module (cli.md / capability-organization.md, L-01 and API-01).
// Non-goals: ban task names in host journeys, identity injection or test code,
// or replace the existing public-contract and dependency-direction guards.
func TestSnapshotCLILeafResponsibilitiesStayWithModule(t *testing.T) {
	root := moduleRoot(t)
	files, err := os.ReadDir(filepath.Join(root, "cli"))
	if err != nil {
		t.Fatal(err)
	}
	owned := snapshotCLILeafOwnership(snapshotcli.Commands())
	seen := map[string]bool{}
	for _, entry := range files {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := "cli/" + entry.Name()
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, path), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		signals, boundaries := snapshotCLIHostOwnershipSignals(path, file, owned)
		for name := range boundaries {
			seen[name] = true
		}
		for _, signal := range signals {
			t.Errorf("%s: %s", path, signal)
		}
	}
	for _, boundary := range []string{"command surface", "helpEntries", "helpGroups", "applyPositionals", "runRemoteRequest", "encodeCLIProduct"} {
		if !seen[boundary] {
			t.Errorf("Snapshot CLI ownership guard cannot find host boundary %s; update the guard with any deliberate assembly change", boundary)
		}
	}
}

type snapshotCLILeaves struct{ paths, handlers map[string]bool }

func snapshotCLILeafOwnership(commands []snapshotcli.Command) snapshotCLILeaves {
	owned := snapshotCLILeaves{paths: map[string]bool{}, handlers: map[string]bool{}}
	for _, command := range commands {
		owned.paths[command.Path] = true
		owned.handlers[command.Handler] = true
	}
	return owned
}

func snapshotCLIHostOwnershipSignals(path string, file *ast.File, owned snapshotCLILeaves) ([]string, map[string]bool) {
	if filepath.ToSlash(filepath.Dir(path)) != "cli" || strings.HasSuffix(path, "_test.go") {
		return nil, nil
	}
	var signals []string
	seen := map[string]bool{}
	checkMaps := func(name string, node ast.Node) {
		signals = append(signals, snapshotCLIExplicitMapSignals(name, node, owned.paths)...)
	}
	for _, declaration := range file.Decls {
		switch item := declaration.(type) {
		case *ast.GenDecl:
			for _, raw := range item.Specs {
				spec, ok := raw.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range spec.Names {
					if !snapshotCLIRegistrationName(name.Name) && name.Name != "helpGroups" {
						continue
					}
					seen[snapshotCLIHostBoundary(name.Name)] = true
					if i < len(spec.Values) {
						checkMaps(name.Name, spec.Values[i])
					}
				}
			}
		case *ast.FuncDecl:
			name := item.Name.Name
			switch {
			case snapshotCLIRegistrationName(name) || strings.HasPrefix(name, "host") && strings.HasSuffix(name, "HelpEntries"):
				seen[snapshotCLIHostBoundary(name)] = true
				checkMaps(name, item.Body)
			case name == "applyPositionals":
				seen[name] = true
				signals = append(signals, snapshotCLIHardcodedBranches(item, file, owned.handlers, false)...)
			case name == "runRemoteRequest" || name == "encodeCLIProduct" || name == "shapeCLIProduct":
				seen[name] = true
				signals = append(signals, snapshotCLIHardcodedBranches(item, file, owned.paths, true)...)
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assignment.Lhs {
			index, ok := lhs.(*ast.IndexExpr)
			if !ok {
				continue
			}
			name, ok := index.X.(*ast.Ident)
			if !ok || !snapshotCLIRegistrationName(name.Name) {
				continue
			}
			if leaf, literal := snapshotCLIStaticString(index.Index); literal && owned.paths[leaf] {
				signals = append(signals, fmt.Sprintf("%s explicitly registers Snapshot leaf %q", name.Name, leaf))
			}
		}
		return true
	})
	return signals, seen
}

func snapshotCLIRegistrationName(name string) bool {
	return name == "cliSurface" || name == "publicCommandSurface" || name == "helpEntries"
}

func snapshotCLIHostBoundary(name string) string {
	if name == "cliSurface" || name == "publicCommandSurface" {
		return "command surface"
	}
	return name
}

func snapshotCLIExplicitMapSignals(name string, node ast.Node, owned map[string]bool) []string {
	var signals []string
	if node == nil {
		return signals
	}
	ast.Inspect(node, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if _, ok := literal.Type.(*ast.MapType); !ok {
			return true
		}
		for _, raw := range literal.Elts {
			entry, ok := raw.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, static := snapshotCLIStaticString(entry.Key)
			if !static {
				continue
			}
			if name == "helpGroups" {
				if snapshotCLIPureModuleList(entry.Value, owned) {
					signals = append(signals, fmt.Sprintf("helpGroups handwrites pure Snapshot group %q", key))
				}
			} else if owned[key] {
				signals = append(signals, fmt.Sprintf("%s explicitly registers Snapshot leaf %q", name, key))
			}
		}
		return true
	})
	return signals
}

func snapshotCLIPureModuleList(value ast.Expr, owned map[string]bool) bool {
	list, ok := value.(*ast.CompositeLit)
	if !ok || len(list.Elts) == 0 {
		return false
	}
	for _, value := range list.Elts {
		leaf, static := snapshotCLIStaticString(value)
		if !static || !owned[leaf] {
			return false
		}
	}
	return true
}

func snapshotCLIHardcodedBranches(function *ast.FuncDecl, file *ast.File, owned map[string]bool, prefixes bool) []string {
	parameter := ""
	for _, field := range function.Type.Params.List {
		if typ, ok := field.Type.(*ast.Ident); ok && typ.Name == "string" && len(field.Names) > 0 {
			parameter = field.Names[0].Name
			break
		}
	}
	if parameter == "" || function.Body == nil {
		return nil
	}
	isParameter := func(expr ast.Expr) bool { ident, ok := expr.(*ast.Ident); return ok && ident.Name == parameter }
	imports := snapshotImportNames(file)
	var signals []string
	report := func(value string) {
		signals = append(signals, fmt.Sprintf("%s hardcodes Snapshot leaf branch %q", function.Name.Name, value))
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch item := node.(type) {
		case *ast.SwitchStmt:
			if !isParameter(item.Tag) {
				break
			}
			for _, raw := range item.Body.List {
				for _, expr := range raw.(*ast.CaseClause).List {
					if value, static := snapshotCLIStaticString(expr); static && owned[value] {
						report(value)
					}
				}
			}
		case *ast.BinaryExpr:
			if item.Op != token.EQL && item.Op != token.NEQ {
				break
			}
			for _, pair := range [][2]ast.Expr{{item.X, item.Y}, {item.Y, item.X}} {
				if value, static := snapshotCLIStaticString(pair[1]); isParameter(pair[0]) && static && owned[value] {
					report(value)
				}
			}
		case *ast.CallExpr:
			selector, ok := item.Fun.(*ast.SelectorExpr)
			if !prefixes || !ok || selector.Sel.Name != "HasPrefix" || len(item.Args) != 2 || !isParameter(item.Args[0]) {
				break
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok || imports[pkg.Name] != "strings" {
				break
			}
			prefix, static := snapshotCLIStaticString(item.Args[1])
			if !static || prefix == "" {
				break
			}
			for leaf := range owned {
				if leaf == prefix || strings.HasPrefix(leaf, strings.TrimSuffix(prefix, " ")+" ") {
					report(prefix)
					break
				}
			}
		}
		return true
	})
	return signals
}

func snapshotCLIStaticString(expr ast.Expr) (string, bool) {
	switch value := expr.(type) {
	case *ast.BasicLit:
		if value.Kind == token.STRING {
			text, err := strconv.Unquote(value.Value)
			return text, err == nil
		}
	case *ast.ParenExpr:
		return snapshotCLIStaticString(value.X)
	case *ast.BinaryExpr:
		if value.Op == token.ADD {
			left, lhs := snapshotCLIStaticString(value.X)
			right, rhs := snapshotCLIStaticString(value.Y)
			return left + right, lhs && rhs
		}
	}
	return "", false
}

func TestSnapshotCLIHostOwnershipGuardRejectsScatteredLeafRules(t *testing.T) {
	owned := snapshotCLILeafOwnership(snapshotcli.Commands())
	for _, command := range snapshotcli.Commands() {
		for _, fixture := range []struct{ name, source, signal string }{
			{"surface", fmt.Sprintf("var cliSurface = composeCLICommands(Commands(), map[string]surface{%q: {}})", command.Path), "cliSurface"},
			{"public surface", fmt.Sprintf("var publicCommandSurface = map[string]surface{%q: {}}", command.Path), "publicCommandSurface"},
			{"help", fmt.Sprintf("var helpEntries = mergeHelpEntries(moduleHelp(), map[string]entry{%q: {}})", command.Path), "helpEntries"},
			{"help helper", fmt.Sprintf("func hostOtherHelpEntries() map[string]entry { return map[string]entry{%q: {}} }", command.Path), "hostOtherHelpEntries"},
			{"operand", fmt.Sprintf("func applyPositionals(handler string) { switch handler { case %q: bindLeaf() } }", command.Handler), "applyPositionals"},
			{"dispatch", fmt.Sprintf("func runRemoteRequest(path string) { if %q == path { runLeaf() } }", command.Path), "runRemoteRequest"},
			{"output", fmt.Sprintf("func encodeCLIProduct(path string) { if path != %q { shapeLeaf() } }", command.Path), "encodeCLIProduct"},
			{"output helper", fmt.Sprintf("func shapeCLIProduct(path string) { switch path { case %q: shapeLeaf() } }", command.Path), "shapeCLIProduct"},
			{"group", fmt.Sprintf("var helpGroups = composeSnapshotHelpGroups(map[string][]string{\"module\": {%q}})", command.Path), "helpGroups"},
			{"late registration", fmt.Sprintf("func addLeaf() { helpEntries[%q] = entry{} }", command.Path), "helpEntries"},
		} {
			t.Run(command.Path+"/"+fixture.name, func(t *testing.T) {
				assertSnapshotCLIOwnershipFixture(t, "cli/host.go", fixture.source, owned, fixture.signal)
			})
		}
	}
	t.Run("family prefix", func(t *testing.T) {
		assertSnapshotCLIOwnershipFixture(t, "cli/host.go", `import text "strings"
func runRemoteRequest(path string) { if text.HasPrefix(path, "dataset" + " ") { runLeaf() } }`, owned, "runRemoteRequest")
	})
}

func TestSnapshotCLIHostOwnershipGuardAllowsAssemblyAndHostContext(t *testing.T) {
	owned := snapshotCLILeafOwnership(snapshotcli.Commands())
	source := `
var cliSurface = composeCLICommands(Commands(), map[string]surface{"create": {}})
var helpEntries = mergeHelpEntries(moduleHelp(), hostWriterHelpEntries())
func hostWriterHelpEntries() map[string]entry { return map[string]entry{"writer merge": {}} }
var helpGroups = composeSnapshotHelpGroups(map[string][]string{"composition": {"create"}, "global journey": {"show", "read"}})
var composeHelp = "kc show then kc dataset define and kc grant list"
func applyPositionals(handler string) { if entry, module := snapshotCommandByHandler(handler); module { bindSnapshotOperands(entry) } }
func runRemoteRequest(path string) { if snapshotCommandPath(path) { executeSnapshotCommand(path) }; switch path { case "pin", "pin check", "create": runHost() } }
func encodeCLIProduct(path string) { if path == "read" { shapeHost() } }
func executeSnapshotCommand(path string) { if path == "grant list" { injectCurrentIdentity() }; executeModule(path) }
func dispatchCLI(path string) { if path == "grant list" { injectCurrentIdentity() } }
`
	for _, path := range []string{"cli/surface.go", "cli/snapshot_adapter.go"} {
		t.Run(path, func(t *testing.T) { assertSnapshotCLIOwnershipFixture(t, path, source, owned, "") })
	}
	for _, path := range []string{"cli/fixture_test.go", "cli/integration_test.go", "internal/testkit/cli.go"} {
		t.Run(path, func(t *testing.T) {
			assertSnapshotCLIOwnershipFixture(t, path, `var helpEntries = map[string]entry{"show": {}}; func applyPositionals(handler string) { switch handler { case "dataset-define": prepareFixture() } }`, owned, "")
		})
	}
}

func assertSnapshotCLIOwnershipFixture(t *testing.T, path, source string, owned snapshotCLILeaves, want string) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, "package cli\n"+source, 0)
	if err != nil {
		t.Fatal(err)
	}
	signals, _ := snapshotCLIHostOwnershipSignals(path, file, owned)
	if want == "" {
		if len(signals) != 0 {
			t.Fatalf("host assembly or fixture was rejected: %v", signals)
		}
	} else if len(signals) != 1 || !strings.Contains(signals[0], want) {
		t.Fatalf("ownership regression must be identified precisely as %q, got %v", want, signals)
	}
}
