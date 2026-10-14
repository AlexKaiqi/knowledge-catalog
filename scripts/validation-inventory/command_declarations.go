package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strconv"
)

func namedValue(file *ast.File, name string) (ast.Expr, error) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != name {
				continue
			}
			if len(value.Values) != 1 {
				return nil, fmt.Errorf("%s must have one static value", name)
			}
			return value.Values[0], nil
		}
	}
	return nil, fmt.Errorf("%s not found", name)
}

func moduleCommandsCall(file *ast.File, expr ast.Expr, module string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Commands" {
		return false
	}
	base, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if path != "kc/"+module {
			continue
		}
		alias := "cli"
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		return base.Name == alias
	}
	return false
}

func usesSnapshotCommands(file *ast.File) bool {
	expr, _ := namedValue(file, "cliSurface")
	_, modules, err := commandComposition(file, expr)
	return err == nil && modules["snapshot/cli"]
}

// commandComposition recognizes only the explicit owned module assemblers.
// Dynamic registration and foreign Commands calls cannot change the denominator.
func commandComposition(file *ast.File, expr ast.Expr) (ast.Expr, map[string]bool, error) {
	modules := map[string]bool{}
	for {
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			return expr, modules, nil
		}
		function, named := call.Fun.(*ast.Ident)
		if !named || len(call.Args) != 2 {
			return nil, nil, fmt.Errorf("CLI composition must explicitly assemble owned Commands and a literal KC map")
		}
		module := ""
		switch function.Name {
		case "composeCLICommands":
			module = "snapshot/cli"
		case "composeObjectCLICommands":
			module = "object/cli"
		case "composeDiscoveryCLICommands":
			module = "discovery/cli"
		case "composeDeclarativeIndexCLICommands":
			module = "declarativeindex/cli"
		case "composeLiveAccessCLICommands":
			module = "liveaccess/cli"
		default:
			return nil, nil, fmt.Errorf("unrecognized CLI composition %s", function.Name)
		}
		if modules[module] || !moduleCommandsCall(file, call.Args[0], module) {
			return nil, nil, fmt.Errorf("CLI composition requires one static %s Commands contribution", module)
		}
		modules[module] = true
		expr = call.Args[1]
	}
}

func cliDeclarations(fset *token.FileSet, file *ast.File, path string) ([]commandEntry, error) {
	expr, err := namedValue(file, "cliSurface")
	if err != nil {
		return nil, err
	}
	expr, _, err = commandComposition(file, expr)
	if err != nil {
		return nil, err
	}
	mapping, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("cliSurface is no longer a literal map or explicit module composition")
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

func snapshotCommandDeclarations(fset *token.FileSet, file *ast.File, path string) ([]commandEntry, error) {
	expr, err := namedValue(file, "commandEntries")
	if err != nil {
		return nil, err
	}
	list, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("module commandEntries must be a literal list")
	}
	entries := []commandEntry{}
	for _, element := range list.Elts {
		fields, ok := element.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("unsupported Snapshot CLI command")
		}
		entry := commandEntry{File: path, Line: fset.Position(element.Pos()).Line}
		seen := map[string]bool{}
		for _, field := range fields.Elts {
			pair, ok := field.(*ast.KeyValueExpr)
			if !ok {
				return nil, fmt.Errorf("snapshot command must use named fields")
			}
			key, ok := pair.Key.(*ast.Ident)
			if !ok || seen[key.Name] {
				return nil, fmt.Errorf("invalid or duplicate Snapshot command field")
			}
			seen[key.Name] = true
			if key.Name != "Path" && key.Name != "Action" {
				continue
			}
			value, ok := literal(pair.Value)
			if !ok || value == "" {
				return nil, fmt.Errorf("snapshot command %s must be a nonempty literal", key.Name)
			}
			if key.Name == "Path" {
				entry.Command, entry.Line = value, fset.Position(pair.Pos()).Line
			} else {
				entry.Action = value
			}
		}
		if entry.Command == "" || entry.Action == "" {
			return nil, fmt.Errorf("snapshot command lacks its path or action")
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func validateCommandInventory(commands []commandEntry) error {
	seen := map[string]commandEntry{}
	for _, entry := range commands {
		if previous, exists := seen[entry.Command]; exists {
			return fmt.Errorf("duplicate command %q in %s:%d and %s:%d", entry.Command, previous.File, previous.Line, entry.File, entry.Line)
		}
		seen[entry.Command] = entry
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].Command < commands[j].Command })
	return nil
}
