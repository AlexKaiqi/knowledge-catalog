package arch_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// CLI and capability-organization own this boundary: root CLI assembles module
// declarations and authenticated transports. Embedded execution belongs to its
// test harness; private compatibility copies do not become a second owner.
func TestRootCLIContainsHostAssemblyWithoutCompatibilityExecutors(t *testing.T) {
	for path, file := range rootCLISourceFiles(t, false) {
		for _, signal := range rootCLIProductionSignals(path, file) {
			t.Error(signal)
		}
	}
}

func TestRootCLIPrivateFunctionsHaveProductionConsumers(t *testing.T) {
	for _, signal := range rootCLIPrivateFunctionSignals(rootCLISourceFiles(t, false)) {
		t.Error(signal)
	}
}

func rootCLIPrivateFunctionSignals(files map[string]*ast.File) []string {
	production := map[string]*ast.File{}
	for path, file := range files {
		if filepath.ToSlash(filepath.Dir(path)) == "cli" && !strings.HasSuffix(path, "_test.go") {
			production[path] = file
		}
	}
	functions := executorFunctions(production, "cli")
	for name, fn := range functions {
		if fn.declaration.Name.IsExported() || name == "init" || name == "_" {
			delete(functions, name)
		}
	}
	referenced := map[string]bool{}
	for _, file := range production {
		ignored, imports := rootCLINonReferenceNames(file), snapshotImportNames(file)
		for _, declaration := range file.Decls {
			origin, _ := declaration.(*ast.FuncDecl)
			ast.Inspect(declaration, func(node ast.Node) bool {
				id, ok := node.(*ast.Ident)
				if !ok || ignored[id] || imports[id.Name] != "" {
					return true
				}
				fn, exists := functions[id.Name]
				if !exists || fn.declaration == origin {
					return true
				}
				// Within a file, parser scope resolution distinguishes the actual
				// package function from local variables, parameters and type names.
				// Cross-file package references remain unresolved until type checking.
				if id.Obj == nil || id.Obj.Kind == ast.Fun && id.Obj.Decl == fn.declaration {
					referenced[id.Name] = true
				}
				return true
			})
		}
	}
	var signals []string
	for path, file := range production {
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if ok && functions[fn.Name.Name].declaration == fn && !referenced[fn.Name.Name] {
				signals = append(signals, path+": private "+fn.Name.Name+" has no other production consumer; test-only calls do not own production helpers")
			}
		}
	}
	slices.Sort(signals)
	return signals
}

func rootCLINonReferenceNames(file *ast.File) map[*ast.Ident]bool {
	ignored := map[*ast.Ident]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.FuncDecl:
			ignored[value.Name] = true
		case *ast.SelectorExpr:
			ignored[value.Sel] = true
		case *ast.Field:
			for _, name := range value.Names {
				ignored[name] = true
			}
		case *ast.KeyValueExpr:
			if name, ok := value.Key.(*ast.Ident); ok {
				ignored[name] = true
			}
		}
		return true
	})
	return ignored
}

func TestRootCLIPrivateFunctionGuardDistinguishesProductionReferences(t *testing.T) {
	for _, fixture := range []struct {
		name, definition, use, usePath string
		rejected                       bool
	}{
		{"declaration alone", "func helper(){}", "", "cli/host.go", true},
		{"self recursion", "func helper(){helper()}", "", "cli/host.go", true},
		{"self callback", "func helper(){consume(helper)}", "", "cli/host.go", true},
		{"test-only reference", "func helper(){}", "func TestHelper(){helper()}", "cli/host_test.go", true},
		{"string is not a consumer", "func helper(){}", `import "fmt";func Run(){fmt.Sprintf("helper")}`, "cli/host.go", true},
		{"selector member is not a function", "func helper(){}", "func Run(remote Remote){remote.helper()}", "cli/host.go", true},
		{"local function shadows package", "func helper(){}", "func Run(){helper:=func(){};helper()}", "cli/host.go", true},
		{"parameter shadows package", "func helper(){}", "func Run(helper func()){helper()}", "cli/host.go", true},
		{"local type shadows package", "func helper(){}", "func Run(){type helper int;_ = helper(1)}", "cli/host.go", true},
		{"type parameter shadows package", "func helper(){}", "func Run[helper any](value helper){_ = value}", "cli/host.go", true},
		{"import shadows package", "func helper(){}", `import helper "fmt";func Run(){helper.Println("value")}`, "cli/host.go", true},
		{"struct key is not a consumer", "func helper(){}", "func Run(){_ = Config{helper:true}}", "cli/host.go", true},
		{"cross-file caller", "func helper(){}", "func Run(){helper()}", "cli/host.go", false},
		{"callback registration", "func helper(){}", "var callbacks=[]func(){helper}", "cli/host.go", false},
		{"generic callback", "func helper[T any](){}", "var callback=helper[int]", "cli/host.go", false},
		{"multiple generic arguments", "func helper[T,U any](){}", "var callback=helper[int,string]", "cli/host.go", false},
		{"receiver base is a call", "func helper() Host{return Host{}}", "func Run(){helper().Serve()}", "cli/host.go", false},
		{"method consumes package helper", "func helper(){}", "func(Host) Serve(){helper()}", "cli/host.go", false},
		{"init is a runtime entry", "func init(){}", "", "cli/host.go", false},
		{"init consumes private helper", "func helper(){}", "func init(){helper()}", "cli/host.go", false},
		{"interface methods are outside scope", "func(Host) helper(){}", "", "cli/host.go", false},
		{"public function is outside scope", "func Helper(){}", "", "cli/host.go", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			files := map[string]*ast.File{"cli/private.go": declarativeIndexGuardFixture(t, "cli/private.go", "package cli;"+fixture.definition)}
			if fixture.use != "" {
				files[fixture.usePath] = declarativeIndexGuardFixture(t, fixture.usePath, "package cli;"+fixture.use)
			}
			signals := rootCLIPrivateFunctionSignals(files)
			if (len(signals) != 0) != fixture.rejected {
				t.Fatalf("private production ownership mismatch: rejected=%t signals=%v", fixture.rejected, signals)
			}
		})
	}
}

func rootCLISourceFiles(t *testing.T, tests bool) map[string]*ast.File {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(moduleRoot(t), "cli", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") != tests {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files["cli/"+filepath.Base(path)] = file
	}
	return files
}

func rootCLIProductionSignals(path string, file *ast.File) []string {
	if filepath.ToSlash(filepath.Dir(path)) != "cli" || strings.HasSuffix(path, "_test.go") {
		return nil
	}
	var signals []string
	hasSymbol := false
	for _, declaration := range file.Decls {
		switch item := declaration.(type) {
		case *ast.FuncDecl:
			hasSymbol = hasSymbol || item.Name.Name != "_"
			if rootCLIEmptyRegistry(item) {
				signals = append(signals, path+": "+item.Name.Name+" retains an always-nil registry")
			}
			if rootCLIForwardingFunction(item, file) {
				signals = append(signals, path+": private "+item.Name.Name+" merely forwards an owner's function")
			}
		case *ast.GenDecl:
			for _, raw := range item.Specs {
				switch spec := raw.(type) {
				case *ast.TypeSpec:
					hasSymbol = hasSymbol || spec.Name.Name != "_"
					if !spec.Name.IsExported() && spec.Assign.IsValid() && rootCLIOwnerSelector(spec.Type, file) != "" {
						signals = append(signals, path+": private "+spec.Name.Name+" retains an owner's type alias")
					}
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						hasSymbol = hasSymbol || name.Name != "_"
						if name.Name != "_" && !name.IsExported() && len(spec.Values) == 1 && rootCLIOwnerSelector(spec.Values[0], file) != "" {
							signals = append(signals, path+": private "+name.Name+" retains an owner's value alias")
						}
					}
				}
			}
		}
	}
	if !hasSymbol {
		signals = append(signals, path+": production file has no owned symbols")
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if binding, ok := node.(*ast.KeyValueExpr); ok {
			if key, ok := binding.Key.(*ast.Ident); ok && key.Name == "Run" && rootCLIOwnerSelector(binding.Value, file) == "kc/invocation" {
				signals = append(signals, path+": embedded execution wiring registers an invocation Run handler in production")
			}
		}
		expression, ok := node.(*ast.SelectorExpr)
		if !ok || rootCLIOwnerSelector(expression, file) != "kc/invocation" {
			return true
		}
		name := expression.Sel.Name
		if name == "Command" || strings.HasPrefix(name, "Verb") || slices.Contains([]string{"ExecuteApplicationOperation", "InvokeApplicationWithTelemetryAtHome"}, name) {
			signals = append(signals, fmt.Sprintf("%s: embedded execution wiring uses invocation.%s in production", path, name))
		}
		return true
	})
	return signals
}

func rootCLIOwnerSelector(expression ast.Expr, file *ast.File) string {
	selector, ok := executorUnparen(expression).(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	alias, ok := selector.X.(*ast.Ident)
	if !ok || alias.Obj != nil {
		return ""
	}
	owner := snapshotImportNames(file)[alias.Name]
	if strings.HasPrefix(owner, "kc/") {
		return owner
	}
	return ""
}

func rootCLIEmptyRegistry(fn *ast.FuncDecl) bool {
	if fn.Body == nil || len(fn.Body.List) != 1 || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		return false
	}
	switch fn.Type.Results.List[0].Type.(type) {
	case *ast.MapType, *ast.FuncType:
		returned, ok := fn.Body.List[0].(*ast.ReturnStmt)
		if ok && len(returned.Results) == 1 {
			id, ok := returned.Results[0].(*ast.Ident)
			return ok && id.Name == "nil"
		}
	}
	return false
}

func rootCLIForwardingFunction(fn *ast.FuncDecl, file *ast.File) bool {
	if fn.Name.IsExported() || fn.Recv != nil || fn.Body == nil || len(fn.Body.List) != 1 {
		return false
	}
	var expression ast.Expr
	switch statement := fn.Body.List[0].(type) {
	case *ast.ReturnStmt:
		if len(statement.Results) != 1 {
			return false
		}
		expression = statement.Results[0]
	case *ast.ExprStmt:
		expression = statement.X
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok || rootCLIOwnerSelector(call.Fun, file) == "" {
		return false
	}
	var parameters []string
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			parameters = append(parameters, name.Name)
		}
	}
	if len(call.Args) != len(parameters) {
		return false
	}
	for i, argument := range call.Args {
		id, ok := argument.(*ast.Ident)
		if !ok || id.Name != parameters[i] {
			return false
		}
	}
	return true
}

func TestRootCLIExistingPublicResultAndFlagTypesRemainOwnedAliases(t *testing.T) {
	want := map[string]string{"FlagValue": "FlagValue", "RunResult": "RunResult"}
	for _, file := range rootCLISourceFiles(t, false) {
		for _, declaration := range file.Decls {
			group, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, raw := range group.Specs {
				spec, ok := raw.(*ast.TypeSpec)
				if !ok || want[spec.Name.Name] == "" {
					continue
				}
				if !spec.Assign.IsValid() || !executorPackageSelector(spec.Type, file, "kc/invoke", want[spec.Name.Name]) {
					t.Errorf("cli.%s must preserve its selected kc/invoke alias", spec.Name.Name)
				}
				delete(want, spec.Name.Name)
			}
		}
	}
	for name := range want {
		t.Errorf("cli.%s public type was removed", name)
	}
}

func TestRootCLIBoundaryGuardRejectsSemanticResidueAndAllowsAssembly(t *testing.T) {
	for _, fixture := range []struct{ name, path, source, signal string }{
		{"empty imports", "cli/new.go", "package cli; import ()", "no owned symbols"},
		{"comment cannot own symbol", "cli/new.go", "package cli // func useful() {}", "no owned symbols"},
		{"nil registry", "cli/new.go", "package cli; func anything() map[string]operation{return nil}", "always-nil registry"},
		{"private type alias", "cli/new.go", `package cli; import owner "kc/invoke"; type view = owner.RunResult`, "owner's type alias"},
		{"private value alias", "cli/new.go", `package cli; import owner "kc/snapshot"; const ref = owner.DefaultRef`, "owner's value alias"},
		{"renamed forwarding", "cli/new.go", `package cli; import owner "kc/invocation"; func convenience(v string)string{return owner.Format(v)}`, "merely forwards"},
		{"void forwarding", "cli/new.go", `package cli; import owner "kc/invocation"; func convenience(v string){owner.Record(v)}`, "merely forwards"},
		{"embedded dispatch", "cli/new.go", `package cli; import owner "kc/invocation"; func dispatch(){owner.ExecuteApplicationOperation()}`, "embedded execution wiring"},
		{"embedded registration", "cli/new.go", `package cli; import owner "kc/invocation"; var commands=map[string]operation{"read":{Run:owner.VerbRead}}`, "embedded execution wiring"},
		{"handler name cannot hide registration", "cli/new.go", `package cli; import owner "kc/invocation"; var commands=map[string]operation{"show":{Run:owner.ReadInventoryState}}`, "embedded execution wiring"},
		{"test harness", "cli/fixture_test.go", `package cli; import owner "kc/invocation"; func dispatch(){owner.ExecuteApplicationOperation()}`, ""},
		{"public type", "cli/types.go", `package cli; import owner "kc/invoke"; type RunResult = owner.RunResult`, ""},
		{"declaration factory", "cli/assembly.go", `package cli; import owner "kc/object/cli"; var commands=owner.Commands()`, ""},
		{"authenticated adapter", "cli/assembly.go", `package cli; import owner "kc/object/cli"; func read(client Transport){owner.New(client,Context{}).Execute()}`, ""},
		{"request carrier", "cli/host.go", `package cli; import owner "kc/invocation"; func login(request *owner.Invocation){ authenticate(request.Context,request.Flags) }`, ""},
		{"owner remains local", "invocation/adapter.go", `package invocation; import owner "kc/invoke"; func convenience(v string)string{return owner.Format(v)}`, ""},
		{"actual local transformation", "cli/host.go", `package cli; import owner "kc/invoke"; func transport(v string)string{return owner.Format(normalize(v))}`, ""},
		{"shadowed import is not owner", "cli/host.go", `package cli; import owner "kc/invoke"; func transport(owner Local,v string)string{return owner.Format(v)}`, ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			file := declarativeIndexGuardFixture(t, fixture.path, fixture.source)
			signals := rootCLIProductionSignals(fixture.path, file)
			if fixture.signal == "" && len(signals) != 0 {
				t.Fatalf("host assembly rejected: %v", signals)
			}
			if fixture.signal != "" && !strings.Contains(strings.Join(signals, "\n"), fixture.signal) {
				t.Fatalf("residue accepted: want %q in %v", fixture.signal, signals)
			}
		})
	}
}

func TestRootCLIModuleUnitAssertionsHaveAHostBoundary(t *testing.T) {
	production, tests := rootCLISourceFiles(t, false), rootCLISourceFiles(t, true)
	for _, signal := range rootCLITestOwnershipSignals(production, tests) {
		t.Error(signal)
	}
}

func rootCLITestOwnershipSignals(production, tests map[string]*ast.File) []string {
	host := executorFunctions(production, "cli")
	for name, fn := range host {
		if rootCLIForwardingFunction(fn.declaration, fn.file) {
			delete(host, name)
		}
	}
	var signals []string
	for path, file := range tests {
		functions := map[string]executorFunction{}
		if file.Name.Name == "cli" {
			functions = executorFunctions(production, "cli")
		}
		for otherPath, other := range tests {
			if other.Name.Name == file.Name.Name {
				for name, fn := range executorFunctions(map[string]*ast.File{otherPath: other}, "cli") {
					functions[name] = fn
				}
			}
		}
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Body == nil {
				continue
			}
			start := executorFunction{fn, file}
			module := rootCLITestReaches(start, functions, func(call *ast.CallExpr, source *ast.File) bool {
				owner := strings.TrimPrefix(rootCLIOwnerSelector(call.Fun, source), "kc/")
				return slices.Contains([]string{"snapshot", "object", "discovery", "declarativeindex", "liveaccess", "invocation", "invoke"}, strings.Split(owner, "/")[0])
			})
			boundary := rootCLITestReaches(start, functions, func(call *ast.CallExpr, source *ast.File) bool {
				if id, ok := call.Fun.(*ast.Ident); ok && source.Name.Name == "cli" && (id.Obj == nil || id.Obj.Kind == ast.Fun) {
					_, owned := host[id.Name]
					return owned
				}
				owner := rootCLIOwnerSelector(call.Fun, source)
				return owner == "kc/cli" || owner == "kc/server" || owner == "kc/home" || executorPackageSelector(call.Fun, source, "net/http/httptest", "NewServer") || rootCLIHandlerHTTPCall(call, source)
			})
			if module && !boundary {
				signals = append(signals, path+": "+fn.Name.Name+" exercises module implementation without a CLI, transport or host assembly boundary; keep its unit assertions with the owner")
			}
		}
	}
	return signals
}

// Follow called helpers and immediate/subtest callbacks. Merely mentioning Run
// in an unused helper or closure is not evidence of an integration boundary.
func rootCLITestReaches(start executorFunction, functions map[string]executorFunction, target executorCallPredicate) bool {
	active := map[*ast.FuncDecl]bool{}
	var visit func(executorFunction) bool
	var body func(*ast.BlockStmt, *ast.File) bool
	visit = func(fn executorFunction) bool {
		if fn.declaration == nil || active[fn.declaration] {
			return false
		}
		active[fn.declaration] = true
		defer delete(active, fn.declaration)
		return body(fn.declaration.Body, fn.file)
	}
	body = func(block *ast.BlockStmt, file *ast.File) bool {
		found := false
		ast.Inspect(block, func(node ast.Node) bool {
			if found {
				return false
			}
			if _, closure := node.(*ast.FuncLit); closure {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if target(call, file) {
				found = true
				return false
			}
			if id, ok := call.Fun.(*ast.Ident); ok && (id.Obj == nil || id.Obj.Kind == ast.Fun) {
				found = visit(functions[id.Name])
			}
			if literal, ok := executorUnparen(call.Fun).(*ast.FuncLit); ok {
				found = body(literal.Body, file)
			}
			if rootCLISubtestCall(call, file) {
				if callback, ok := call.Args[1].(*ast.FuncLit); ok {
					found = body(callback.Body, file)
				}
			}
			return !found
		})
		return found
	}
	return visit(start)
}

func rootCLISubtestCall(call *ast.CallExpr, file *ast.File) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Run" || len(call.Args) != 2 {
		return false
	}
	receiver, ok := selector.X.(*ast.Ident)
	if !ok || receiver.Obj == nil {
		return false
	}
	parameter, ok := receiver.Obj.Decl.(*ast.Field)
	if !ok {
		return false
	}
	pointer, ok := parameter.Type.(*ast.StarExpr)
	return ok && executorPackageSelector(pointer.X, file, "testing", "T")
}

func rootCLIHandlerHTTPCall(call *ast.CallExpr, file *ast.File) bool {
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || method.Sel.Name != "ServeHTTP" || len(call.Args) != 2 {
		return false
	}
	handler := rootCLIInitializedCall(method.X)
	if handler == nil || !strings.HasSuffix(rootCLIOwnerSelector(handler.Fun, file), "/service") {
		return false
	}
	constructor, ok := handler.Fun.(*ast.SelectorExpr)
	if !ok || constructor.Sel.Name != "NewHandler" {
		return false
	}
	for i, constructor := range []string{"NewRecorder", "NewRequest"} {
		argument := rootCLIInitializedCall(call.Args[i])
		if argument == nil || !executorPackageSelector(argument.Fun, file, "net/http/httptest", constructor) {
			return false
		}
	}
	return true
}

func rootCLIInitializedCall(expression ast.Expr) *ast.CallExpr {
	if call, ok := expression.(*ast.CallExpr); ok {
		return call
	}
	name, ok := expression.(*ast.Ident)
	if !ok || name.Obj == nil {
		return nil
	}
	switch declared := name.Obj.Decl.(type) {
	case *ast.AssignStmt:
		for i, expression := range declared.Lhs {
			if id, ok := expression.(*ast.Ident); ok && id.Name == name.Name && i < len(declared.Rhs) {
				call, _ := declared.Rhs[i].(*ast.CallExpr)
				return call
			}
		}
	case *ast.ValueSpec:
		for i, id := range declared.Names {
			if id.Name == name.Name && i < len(declared.Values) {
				call, _ := declared.Values[i].(*ast.CallExpr)
				return call
			}
		}
	}
	return nil
}

func TestRootCLITestOwnershipGuardKeepsUnitAndIntegrationEvidenceDistinct(t *testing.T) {
	production := map[string]*ast.File{"cli/host.go": declarativeIndexGuardFixture(t, "cli/host.go", "package cli; func Run(){};func composeModuleView(){}")}
	for _, fixture := range []struct {
		name, source string
		rejected     bool
	}{
		{"owner unit", `func TestValue(t *testing.T){owner.Validate()}`, true},
		{"unused host helper", `func spare(){Run()};func TestValue(t *testing.T){owner.Validate()}`, true},
		{"unused host closure", `func TestValue(t *testing.T){owner.Validate();_=func(){Run()}}`, true},
		{"shadowed host name", `func TestValue(t *testing.T){Run:=func(){};Run();owner.Validate()}`, true},
		{"unrelated Run method", `func TestValue(t *testing.T){owner.Validate();other.Run("value",func(){Run()})}`, true},
		{"fake HTTP handler", `func TestValue(t *testing.T){owner.Validate();other.ServeHTTP(writer,request)}`, true},
		{"called host", `func TestValue(t *testing.T){Run();owner.Validate()}`, false},
		{"delegation assembly", `func TestValue(t *testing.T){composeModuleView();owner.Validate()}`, false},
		{"called helper", `func check(){Run()};func TestValue(t *testing.T){check();owner.Validate()}`, false},
		{"subtest integration", `func TestValue(t *testing.T){t.Run("value",func(t *testing.T){Run();owner.Validate()})}`, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			path := "cli/integration_test.go"
			source := "package cli; import owner \"kc/object/cli\"; import \"testing\";" + fixture.source
			signals := rootCLITestOwnershipSignals(production, map[string]*ast.File{path: declarativeIndexGuardFixture(t, path, source)})
			if (len(signals) != 0) != fixture.rejected {
				t.Fatalf("module ownership mismatch: rejected=%t signals=%v", fixture.rejected, signals)
			}
		})
	}
	path := "cli/owner_test.go"
	forwarder := declarativeIndexGuardFixture(t, "cli/forward.go", `package cli; import owner "kc/object/cli"; func proxy(){owner.Validate()}`)
	unit := declarativeIndexGuardFixture(t, path, `package cli; import "testing"; func TestValue(t *testing.T){proxy()}`)
	if signals := rootCLITestOwnershipSignals(map[string]*ast.File{"cli/forward.go": forwarder}, map[string]*ast.File{path: unit}); len(signals) == 0 {
		t.Fatal("a private owner forwarder disguised its unit test as host integration")
	}
	for _, source := range []struct {
		body     string
		rejected bool
	}{
		{`handler:=service.NewHandler();writer:=httptest.NewRecorder();request:=httptest.NewRequest();handler.ServeHTTP(writer,request)`, false},
		{`handler:=service.NewHandler();writer:=httptest.NewRecorder();request:=httptest.NewRequest();_ = handler;_ = writer;_ = request`, true},
		{`handler:=service.NewHandler();writer:=httptest.NewRecorder();request:=httptest.NewRequest();other.ServeHTTP(writer,request)`, true},
	} {
		file := declarativeIndexGuardFixture(t, path, `package cli;import "testing";import "net/http/httptest";import service "kc/snapshot/service";func TestHTTP(t *testing.T){`+source.body+`}`)
		signals := rootCLITestOwnershipSignals(production, map[string]*ast.File{path: file})
		if (len(signals) != 0) != source.rejected {
			t.Fatalf("real handler/recorder/request wiring mismatch: rejected=%t signals=%v", source.rejected, signals)
		}
	}
}
