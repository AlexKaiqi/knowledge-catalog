package arch_test

import (
	"go/ast"
	"path/filepath"
)

type executorFunction struct {
	declaration *ast.FuncDecl
	file        *ast.File
}

type executorCallback struct {
	expression ast.Expr
	file       *ast.File
}

type executorCallPredicate func(*ast.CallExpr, *ast.File) bool

func typedExecutorWiringSignals(files map[string]*ast.File) []string {
	var signals []string
	for _, check := range []struct{ verb, group, command, handler, route, pkg, core string }{
		{"VerbRead", "readVerbs", "knowledge-read", "knowledgeRead", "POST /knowledge/v1/objects:read", "kc/liveaccess/service", "ReadRepository"},
		{"VerbSearch", "indexVerbs", "knowledge-search", "knowledgeSearch", "POST /knowledge/v1/search", "kc/discovery/service", "SearchExecutor"},
	} {
		if !executorCLICommand(files, check.group, check.command, check.verb) || !executorHTTPRoute(files, check.handler, check.route, check.command, check.verb) {
			signals = append(signals, "CLI/HTTP route is not bound to invocation."+check.verb)
		}
		functions := executorFunctions(files, "invocation")
		if !executorReaches(functions[check.verb], functions, func(call *ast.CallExpr, file *ast.File) bool {
			if check.verb == "VerbRead" {
				return executorPackageSelector(call.Fun, file, check.pkg, check.core)
			}
			method, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || method.Sel.Name != "Execute" {
				return false
			}
			literal, ok := executorUnparen(method.X).(*ast.CompositeLit)
			return ok && executorPackageSelector(literal.Type, file, check.pkg, check.core)
		}) {
			signals = append(signals, "invocation."+check.verb+" does not call its typed executor "+check.core)
		}
	}
	return signals
}

func executorFunctions(files map[string]*ast.File, owner string) map[string]executorFunction {
	functions := map[string]executorFunction{}
	for path, file := range files {
		if filepath.ToSlash(filepath.Dir(path)) != owner {
			continue
		}
		for _, declaration := range file.Decls {
			if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Recv == nil {
				functions[fn.Name.Name] = executorFunction{fn, file}
			}
		}
	}
	return functions
}

func executorMethod(files map[string]*ast.File, owner, receiver, name string) executorFunction {
	for path, file := range files {
		if filepath.ToSlash(filepath.Dir(path)) == owner {
			if fn := lifecycleMethod(file, receiver, name); fn != nil {
				return executorFunction{fn, file}
			}
		}
	}
	return executorFunction{}
}

func executorCLICommand(files map[string]*ast.File, group, command, verb string) bool {
	fn := executorFunctions(files, "cli")[group]
	if fn.declaration == nil {
		return false
	}
	for _, statement := range fn.declaration.Body.List {
		returned, ok := statement.(*ast.ReturnStmt)
		if !ok || len(returned.Results) != 1 {
			continue
		}
		literal, ok := returned.Results[0].(*ast.CompositeLit)
		if !ok {
			continue
		}
		for _, raw := range literal.Elts {
			entry, ok := raw.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			name, _ := snapshotCLIStaticString(entry.Key)
			if name == command && executorCommandRun(entry.Value, fn.file, verb) {
				return true
			}
		}
	}
	return false
}

func executorHTTPRoute(files map[string]*ast.File, handler, route, command, verb string) bool {
	registered := executorMethod(files, "server", "httpFacade", "registerServiceRoutes")
	bound := executorReaches(registered, nil, func(call *ast.CallExpr, _ *ast.File) bool {
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || method.Sel.Name != "HandleFunc" || len(call.Args) != 2 {
			return false
		}
		name, _ := snapshotCLIStaticString(call.Args[0])
		target, ok := call.Args[1].(*ast.SelectorExpr)
		return name == route && ok && target.Sel.Name == handler && executorReceiver(target.X, registered.declaration)
	})
	fn := executorMethod(files, "server", "httpFacade", handler)
	return bound && executorReaches(fn, nil, func(call *ast.CallExpr, file *ast.File) bool {
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || method.Sel.Name != "executeTyped" || !executorReceiver(method.X, fn.declaration) || len(call.Args) < 5 {
			return false
		}
		name, _ := snapshotCLIStaticString(call.Args[2])
		literal, ok := call.Args[4].(*ast.CompositeLit)
		return name == command && ok && executorPackageSelector(literal.Type, file, "kc/invocation", "Command") && executorCommandRun(literal, file, verb)
	})
}

func executorCommandRun(expression ast.Expr, file *ast.File, verb string) bool {
	literal, ok := expression.(*ast.CompositeLit)
	if !ok {
		return false
	}
	for _, raw := range literal.Elts {
		field, ok := raw.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		name, ok := field.Key.(*ast.Ident)
		if ok && name.Name == "Run" && executorPackageSelector(field.Value, file, "kc/invocation", verb) {
			return true
		}
	}
	return false
}

func executorReceiver(expression ast.Expr, fn *ast.FuncDecl) bool {
	name, ok := expression.(*ast.Ident)
	return ok && fn != nil && fn.Recv != nil && len(fn.Recv.List) == 1 && len(fn.Recv.List[0].Names) == 1 && name.Name == fn.Recv.List[0].Names[0].Name
}

func executorPackageSelector(expression ast.Expr, file *ast.File, pkg, member string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != member {
		return false
	}
	alias, ok := selector.X.(*ast.Ident)
	return ok && alias.Obj == nil && snapshotImportNames(file)[alias.Name] == pkg
}

func executorUnparen(expression ast.Expr) ast.Expr {
	for {
		paren, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = paren.X
	}
}

// Only called helpers and callbacks actually invoked by their callee enter the
// reachability graph. A spare helper or an unused function literal is not proof.
func executorReaches(start executorFunction, functions map[string]executorFunction, target executorCallPredicate) bool {
	active := map[*ast.FuncDecl]bool{}
	var body func(*ast.BlockStmt, *ast.File, map[string]executorCallback) bool
	visit := func(fn executorFunction, arguments []executorCallback) bool {
		if fn.declaration == nil || fn.declaration.Body == nil || active[fn.declaration] {
			return false
		}
		active[fn.declaration] = true
		defer delete(active, fn.declaration)
		bindings, index := map[string]executorCallback{}, 0
		for _, field := range fn.declaration.Type.Params.List {
			for _, name := range field.Names {
				if index < len(arguments) {
					bindings[name.Name] = arguments[index]
				}
				index++
			}
		}
		return body(fn.declaration.Body, fn.file, bindings)
	}
	body = func(block *ast.BlockStmt, file *ast.File, bindings map[string]executorCallback) bool {
		if block == nil {
			return false
		}
		found := false
		ast.Inspect(block, func(node ast.Node) bool {
			if found {
				return false
			}
			if _, literal := node.(*ast.FuncLit); literal {
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
			if literal, ok := executorUnparen(call.Fun).(*ast.FuncLit); ok {
				found = body(literal.Body, file, bindings)
				return !found
			}
			name, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if callback, supplied := bindings[name.Name]; supplied {
				if literal, ok := callback.expression.(*ast.FuncLit); ok {
					found = body(literal.Body, callback.file, nil)
				} else if delegated, ok := callback.expression.(*ast.Ident); ok {
					found = visit(functions[delegated.Name], nil)
				}
				return !found
			}
			if name.Obj != nil && name.Obj.Kind != ast.Fun {
				return true
			}
			arguments := make([]executorCallback, len(call.Args))
			for i, argument := range call.Args {
				arguments[i] = executorCallback{argument, file}
				if forwarded, ok := argument.(*ast.Ident); ok {
					if callback, supplied := bindings[forwarded.Name]; supplied {
						arguments[i] = callback
					}
				}
			}
			found = visit(functions[name.Name], arguments)
			return !found
		})
		return found
	}
	if start.declaration == nil {
		return false
	}
	return visit(start, nil)
}
