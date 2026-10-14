package arch_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func projectionWorkerActivationSignals(path string, file *ast.File) []string {
	var signals []string
	if strings.HasPrefix(path, "home/") {
		ast.Inspect(file, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Start" {
					signals = append(signals, "cold Home must not activate workers; activation belongs to the long-lived Server")
				}
			}
			return true
		})
		return signals
	}
	switch path {
	case "server/serve_handler.go":
		start := lifecycleMethod(file, "ManagedHTTPHandler", "Start")
		if !callsLifecycleMember(start, "activate") {
			return []string{"ManagedHTTPHandler.Start must invoke its root activation callback"}
		}
		return nil
	case "server/serve_close.go":
		begin := lifecycleOnceMemberCalls(lifecycleMethod(file, "ManagedHTTPHandler", "BeginShutdown"), "beginOnce")
		if !begin["stopHome"].IsValid() {
			signals = append(signals, "BeginShutdown must cancel its root background work exactly once")
		}
		close := lifecycleOnceMemberCalls(lifecycleMethod(file, "ManagedHTTPHandler", "Close"), "once")
		if !close["BeginShutdown"].IsValid() || !close["closeHome"].IsValid() || close["BeginShutdown"] >= close["closeHome"] {
			signals = append(signals, "Close must begin shutdown before releasing its root")
		}
		return signals
	case "cli/serve.go":
		if !cliStartsManagedHandlerBeforeListen(file) {
			return []string{"kc serve must activate its managed handler before listening"}
		}
		if !cliBeginsShutdownBeforeDrain(file) {
			return []string{"kc serve must begin managed runtime shutdown before HTTP drain"}
		}
		return nil
	case "server/plugin_help.go":
		help := facadeLifecycleMethod(file, "pluginHelp")
		if help == nil || hasLifecycleCall(help.Body, "Start", "activate", "startProjectionWorker", "readHomeForRequest", "executeTyped") {
			return []string{"static plugin help must not activate a root or enter the runtime task dispatcher"}
		}
		return nil
	case "server/service_invoke.go", "server/snapshot_service.go":
		name := "executeTyped"
		if path == "server/snapshot_service.go" {
			name = "acquireSnapshotResources"
		}
		if !callsLifecycleMember(facadeLifecycleMethod(file, name), "readHomeForRequest") {
			return []string{name + " must acquire the same managed root"}
		}
		return nil
	case "server/serve_facade.go":
	default:
		return nil
	}
	start := facadeLifecycleMethod(file, "startProjectionWorker")
	if start == nil {
		return []string{"Server has no worker activation method"}
	}
	home := lifecycleHomeParameter(start)
	for _, worker := range []string{"Plugins", "ObjectMaintenance"} {
		started := false
		inspectLifecycleCalls(start.Body, func(call *ast.CallExpr) {
			method, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || method.Sel.Name != "Start" {
				return
			}
			field, ok := method.X.(*ast.SelectorExpr)
			if !ok || field.Sel.Name != worker {
				return
			}
			owner, ok := field.X.(*ast.Ident)
			started = started || ok && home != "" && owner.Name == home
		})
		if !started {
			signals = append(signals, "Server must activate the supplied Home's "+worker)
		}
	}
	read := facadeLifecycleMethod(file, "readHomeForRequest")
	for _, home := range []string{"f.readHome", "ws"} {
		if !callsLifecycleActivation(read, home) {
			signals = append(signals, "readHomeForRequest must activate its long-lived Home path "+home)
		}
	}
	var handler *ast.FuncDecl
	for _, raw := range file.Decls {
		fn, ok := raw.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		switch fn.Name.Name {
		case "HTTPHandlerFromConfig", "HTTPHandlerWithOptions", "HTTPHandlerWithTelemetry", "newHTTPHandler", "newHTTPHandlerWithRuntime":
			if hasLifecycleCall(fn.Body, "Start", "startProjectionWorker", "readHomeForRequest") {
				signals = append(signals, "handler construction must remain cold: "+fn.Name.Name)
			}
		}
		if fn.Name.Name == "newHTTPHandlerWithRuntime" {
			handler = fn
		}
	}
	if !managedRootActivationIsBound(handler) {
		signals = append(signals, "cold managed handler must bind its root activation callback")
	}
	if !managedRootStopIsBound(handler) {
		signals = append(signals, "cold managed handler must bind its root stop callback")
	}
	stop := facadeLifecycleMethod(file, "stopReadHome")
	if stop == nil || !hasLifecycleCall(stop.Body, "StopMaintenance") || hasLifecycleCall(stop.Body, "Close") {
		signals = append(signals, "root stop must cancel maintenance while preserving request resources")
	}
	return signals
}

func facadeLifecycleMethod(file *ast.File, name string) *ast.FuncDecl {
	return lifecycleMethod(file, "httpFacade", name)
}

func lifecycleMethod(file *ast.File, receiver, name string) *ast.FuncDecl {
	for _, raw := range file.Decls {
		fn, ok := raw.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		pointer, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if owner, named := pointerTypeName(pointer, ok); named && owner == receiver {
			return fn
		}
	}
	return nil
}

func lifecycleHomeParameter(fn *ast.FuncDecl) string {
	for _, field := range fn.Type.Params.List {
		pointer, ok := field.Type.(*ast.StarExpr)
		if name, named := pointerTypeName(pointer, ok); named && name == "Home" && len(field.Names) == 1 {
			return field.Names[0].Name
		}
	}
	return ""
}

func pointerTypeName(pointer *ast.StarExpr, ok bool) (string, bool) {
	if !ok {
		return "", false
	}
	switch value := pointer.X.(type) {
	case *ast.Ident:
		return value.Name, true
	case *ast.SelectorExpr:
		return value.Sel.Name, true
	default:
		return "", false
	}
}

func callsLifecycleActivation(fn *ast.FuncDecl, home string) bool {
	if fn == nil {
		return false
	}
	found := false
	inspectLifecycleCalls(fn.Body, func(call *ast.CallExpr) {
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || method.Sel.Name != "startProjectionWorker" || len(call.Args) != 1 {
			return
		}
		found = found || lifecycleExpressionName(call.Args[0]) == home
	})
	return found
}

func callsLifecycleMember(fn *ast.FuncDecl, name string) bool {
	if fn == nil || fn.Recv == nil || len(fn.Recv.List[0].Names) != 1 {
		return false
	}
	owner := fn.Recv.List[0].Names[0].Name
	found := false
	inspectLifecycleCalls(fn.Body, func(call *ast.CallExpr) {
		if method, ok := call.Fun.(*ast.SelectorExpr); ok && method.Sel.Name == name {
			found = found || lifecycleExpressionName(method.X) == owner
		}
	})
	return found
}

func lifecycleOnceMemberCalls(fn *ast.FuncDecl, once string) map[string]token.Pos {
	calls := map[string]token.Pos{}
	if fn == nil || fn.Recv == nil || len(fn.Recv.List[0].Names) != 1 {
		return calls
	}
	owner := fn.Recv.List[0].Names[0].Name
	inspectLifecycleCalls(fn.Body, func(call *ast.CallExpr) {
		if lifecycleExpressionName(call.Fun) != owner+"."+once+".Do" || len(call.Args) != 1 {
			return
		}
		if callback, ok := call.Args[0].(*ast.FuncLit); ok {
			inspectLifecycleCalls(callback.Body, func(inner *ast.CallExpr) {
				if method, ok := inner.Fun.(*ast.SelectorExpr); ok && lifecycleExpressionName(method.X) == owner {
					if !calls[method.Sel.Name].IsValid() {
						calls[method.Sel.Name] = inner.Pos()
					}
				}
			})
		}
	})
	return calls
}

func hasLifecycleCall(body *ast.BlockStmt, names ...string) bool {
	found := false
	inspectLifecycleCalls(body, func(call *ast.CallExpr) {
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}
		for _, name := range names {
			found = found || method.Sel.Name == name
		}
	})
	return found
}

func managedRootActivationIsBound(fn *ast.FuncDecl) bool {
	if fn == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok || lifecycleExpressionName(literal.Type) != "ManagedHTTPHandler" {
			return true
		}
		for _, raw := range literal.Elts {
			field, ok := raw.(*ast.KeyValueExpr)
			if !ok || lifecycleExpressionName(field.Key) != "activate" {
				continue
			}
			callback, ok := field.Value.(*ast.FuncLit)
			if ok {
				inspectLifecycleCalls(callback.Body, func(call *ast.CallExpr) {
					found = found || lifecycleExpressionName(call.Fun) == "facade.readHomeForRequest"
				})
			}
		}
		return false
	})
	return found
}

func managedRootStopIsBound(fn *ast.FuncDecl) bool {
	if fn == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok || lifecycleExpressionName(literal.Type) != "ManagedHTTPHandler" {
			return true
		}
		for _, raw := range literal.Elts {
			field, ok := raw.(*ast.KeyValueExpr)
			if ok && lifecycleExpressionName(field.Key) == "stopHome" {
				found = found || lifecycleExpressionName(field.Value) == "facade.stopReadHome"
			}
		}
		return false
	})
	return found
}

func cliStartsManagedHandlerBeforeListen(file *ast.File) bool {
	var run *ast.FuncDecl
	for _, raw := range file.Decls {
		if fn, ok := raw.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "runServe" {
			run = fn
		}
	}
	if run == nil {
		return false
	}
	starters := map[string]bool{}
	var started, listen token.Pos
	ast.Inspect(run.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Rhs) == 1 && len(value.Lhs) >= 1 {
				if assertion, ok := value.Rhs[0].(*ast.TypeAssertExpr); ok && lifecycleExpressionName(assertion.X) == "handler" {
					starters[lifecycleExpressionName(value.Lhs[0])] = true
				}
			}
		case *ast.CallExpr:
			if method, ok := value.Fun.(*ast.SelectorExpr); ok {
				if method.Sel.Name == "Start" && starters[lifecycleExpressionName(method.X)] {
					started = value.Pos()
				}
				if method.Sel.Name == "ListenAndServe" {
					listen = value.Pos()
				}
			}
		}
		return true
	})
	return started.IsValid() && listen.IsValid() && started < listen
}

func cliBeginsShutdownBeforeDrain(file *ast.File) bool {
	var run *ast.FuncDecl
	for _, raw := range file.Decls {
		if fn, ok := raw.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "runServe" {
			run = fn
		}
	}
	if run == nil {
		return false
	}
	aliases := map[string]bool{"handler": true}
	var begun, drain token.Pos
	ast.Inspect(run.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Rhs) == 1 && len(value.Lhs) >= 1 {
				if assertion, ok := value.Rhs[0].(*ast.TypeAssertExpr); ok && lifecycleExpressionName(assertion.X) == "handler" {
					aliases[lifecycleExpressionName(value.Lhs[0])] = true
				}
			}
		case *ast.CallExpr:
			if method, ok := value.Fun.(*ast.SelectorExpr); ok {
				if method.Sel.Name == "BeginShutdown" && aliases[lifecycleExpressionName(method.X)] {
					begun = value.Pos()
				}
				if lifecycleExpressionName(value.Fun) == "server.Shutdown" {
					drain = value.Pos()
				}
			}
		}
		return true
	})
	return begun.IsValid() && drain.IsValid() && begun < drain
}

func TestServeShutdownCancelsWorkersBeforeHTTPDrain(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "cli", "serve.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !cliBeginsShutdownBeforeDrain(file) {
		t.Fatal("kc serve must begin managed runtime shutdown before HTTP drain; deferred Close cannot cancel a background lock while Shutdown waits")
	}
}

func lifecycleExpressionName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return lifecycleExpressionName(value.X) + "." + value.Sel.Name
	case *ast.BasicLit:
		text, _ := strconv.Unquote(value.Value)
		return text
	default:
		return ""
	}
}

func inspectLifecycleCalls(body *ast.BlockStmt, visit func(*ast.CallExpr)) {
	if body == nil {
		return
	}
	ast.Inspect(body, func(node ast.Node) bool {
		// Merely defining a callback does not activate a worker.
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		if call, ok := node.(*ast.CallExpr); ok {
			visit(call)
		}
		return true
	})
}

func TestProjectionWorkerLifecycleGuardRejectsColdStartAndMissingActivation(t *testing.T) {
	serve := `package server
func newHTTPHandlerWithRuntime(opened *Home) { return &ManagedHTTPHandler{stopHome: facade.stopReadHome, activate: func() error { _, err := facade.readHomeForRequest(); return err }} }
func (f *httpFacade) readHomeForRequest() { f.startProjectionWorker(f.readHome); f.startProjectionWorker(ws) }
func (f *httpFacade) stopReadHome() { opened.StopMaintenance() }
func (f *httpFacade) startProjectionWorker(workspace *Home) {
    workspace.Plugins.Start(ctx)
    workspace.ObjectMaintenance.Start(ctx)
}`
	for _, tc := range []struct {
		name, path, source, signal string
	}{
		{"cold assembly", "home/home.go", "package home; func Open() { configure() }", ""},
		{"cold plugin start", "home/home.go", "package home; func Open() { ws.Plugins.Start(ctx) }", "cold Home"},
		{"cold cache start in helper", "home/assembly.go", "package home; func assemble() { ws.ObjectMaintenance.Start(ctx) }", "cold Home"},
		{"long-lived assembly", "server/serve_facade.go", serve, ""},
		{"plugin activation removed", "server/serve_facade.go", strings.Replace(serve, "workspace.Plugins.Start(ctx)", "", 1), "Home's Plugins"},
		{"cache activation removed", "server/serve_facade.go", strings.Replace(serve, "workspace.ObjectMaintenance.Start(ctx)", "", 1), "Home's ObjectMaintenance"},
		{"comment is not activation", "server/serve_facade.go", strings.Replace(serve, "workspace.Plugins.Start(ctx)", "// workspace.Plugins.Start(ctx)", 1), "Home's Plugins"},
		{"callback is not activation", "server/serve_facade.go", strings.Replace(serve, "workspace.Plugins.Start(ctx)", "_ = func() { workspace.Plugins.Start(ctx) }", 1), "Home's Plugins"},
		{"other Home is not owned worker", "server/serve_facade.go", strings.Replace(serve, "workspace.Plugins.Start(ctx)", "other.Plugins.Start(ctx)", 1), "Home's Plugins"},
		{"new root does not activate", "server/serve_facade.go", strings.Replace(serve, "f.startProjectionWorker(ws)", "", 1), "Home path ws"},
		{"supplied root does not activate", "server/serve_facade.go", strings.Replace(serve, "f.startProjectionWorker(f.readHome)", "", 1), "Home path f.readHome"},
		{"handler constructor activates", "server/serve_facade.go", strings.Replace(serve, "return &ManagedHTTPHandler", "facade.startProjectionWorker(opened); return &ManagedHTTPHandler", 1), "construction must remain cold"},
		{"configured handler constructor activates", "server/serve_facade.go", serve + "\nfunc HTTPHandlerFromConfig() { ws.Plugins.Start(ctx) }", "construction must remain cold: HTTPHandlerFromConfig"},
		{"managed callback disconnected", "server/serve_facade.go", strings.Replace(serve, "_, err := facade.readHomeForRequest(); return err", "return nil", 1), "bind its root activation callback"},
		{"managed stop callback disconnected", "server/serve_facade.go", strings.Replace(serve, "stopHome: facade.stopReadHome", "stopHome: other.stopReadHome", 1), "bind its root stop callback"},
		{"root stop releases resources", "server/serve_facade.go", strings.Replace(serve, "opened.StopMaintenance()", "opened.Close()", 1), "preserving request resources"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), tc.path, tc.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			signals := projectionWorkerActivationSignals(tc.path, file)
			if tc.signal == "" {
				if len(signals) != 0 {
					t.Fatalf("valid lifecycle refused: %v", signals)
				}
			} else if !strings.Contains(strings.Join(signals, "\n"), tc.signal) {
				t.Fatalf("lifecycle violation lost: want=%q signals=%v", tc.signal, signals)
			}
		})
	}
}

func TestManagedHandlerLifecycleGuardRejectsDisconnectedActivation(t *testing.T) {
	managed := `package server; func (h *ManagedHTTPHandler) Start() error { return h.activate() }`
	cli := `package cli; func runServe() { starter, ok := handler.(interface{ Start() error }); if ok { starter.Start() }; server.ListenAndServe(); stopper, ok := handler.(interface{ BeginShutdown() }); if ok { stopper.BeginShutdown() }; server.Shutdown(ctx) }`
	for _, tc := range []struct {
		name, path, source, signal string
	}{
		{"managed activation", "server/serve_handler.go", managed, ""},
		{"managed callback omitted", "server/serve_handler.go", strings.Replace(managed, "h.activate()", "nil", 1), "root activation callback"},
		{"managed activates another handler", "server/serve_handler.go", strings.Replace(managed, "h.activate()", "other.activate()", 1), "root activation callback"},
		{"serve activates before listen", "cli/serve.go", cli, ""},
		{"serve activation omitted", "cli/serve.go", strings.Replace(cli, "starter.Start()", "", 1), "before listening"},
		{"serve activation delayed", "cli/serve.go", strings.Replace(cli, "if ok { starter.Start() }; server.ListenAndServe()", "server.ListenAndServe(); if ok { starter.Start() }", 1), "before listening"},
		{"serve cancellation omitted", "cli/serve.go", strings.Replace(cli, "stopper.BeginShutdown()", "", 1), "before HTTP drain"},
		{"serve cancellation delayed", "cli/serve.go", strings.Replace(cli, "if ok { stopper.BeginShutdown() }; server.Shutdown(ctx)", "server.Shutdown(ctx); if ok { stopper.BeginShutdown() }", 1), "before HTTP drain"},
		{"managed begins shutdown", "server/serve_close.go", `package server; func (h *ManagedHTTPHandler) BeginShutdown() { h.beginOnce.Do(func(){ h.stopHome() }) }; func (h *ManagedHTTPHandler) Close() { h.once.Do(func(){ h.BeginShutdown(); h.closeHome() }) }`, ""},
		{"managed cancellation disconnected", "server/serve_close.go", `package server; func (h *ManagedHTTPHandler) BeginShutdown() { h.beginOnce.Do(func(){}) }; func (h *ManagedHTTPHandler) Close() { h.once.Do(func(){ h.BeginShutdown(); h.closeHome() }) }`, "cancel its root background"},
		{"managed releases before cancellation", "server/serve_close.go", `package server; func (h *ManagedHTTPHandler) BeginShutdown() { h.beginOnce.Do(func(){ h.stopHome() }) }; func (h *ManagedHTTPHandler) Close() { h.once.Do(func(){ h.closeHome(); h.BeginShutdown() }) }`, "before releasing its root"},
		{"help stays static", "server/plugin_help.go", `package server; func (f *httpFacade) pluginHelp() { explain() }`, ""},
		{"help activates", "server/plugin_help.go", `package server; func (f *httpFacade) pluginHelp() { f.readHomeForRequest() }`, "static plugin help"},
		{"help enters runtime dispatcher", "server/plugin_help.go", `package server; func (f *httpFacade) pluginHelp() { f.executeTyped() }`, "static plugin help"},
		{"typed task shares root", "server/service_invoke.go", `package server; func (f *httpFacade) executeTyped() { f.readHomeForRequest() }`, ""},
		{"typed task loses root", "server/service_invoke.go", `package server; func (f *httpFacade) executeTyped() { other.readHomeForRequest() }`, "same managed root"},
		{"Snapshot task shares root", "server/snapshot_service.go", `package server; func (f *httpFacade) acquireSnapshotResources() { f.readHomeForRequest() }`, ""},
		{"Snapshot task loses root", "server/snapshot_service.go", `package server; func (f *httpFacade) acquireSnapshotResources() {}`, "same managed root"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), tc.path, tc.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			signals := projectionWorkerActivationSignals(tc.path, file)
			if tc.signal == "" {
				if len(signals) != 0 {
					t.Fatalf("valid managed lifecycle refused: %v", signals)
				}
			} else if !strings.Contains(strings.Join(signals, "\n"), tc.signal) {
				t.Fatalf("managed lifecycle violation lost: want=%q signals=%v", tc.signal, signals)
			}
		})
	}
}
