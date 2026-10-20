package arch_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// AddressKey and Key methods build internal map keys joined by a control
// separator. Messages that reach callers name an address through AddressLabel
// and a field through its display form; a key leaking into an error or reason
// string shows up as \u001f in JSON output.
func TestMessagesNameAddressesByLabelNotKey(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	var signals []string
	checked := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel != "." && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" || entry.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		checked++
		signals = append(signals, addressKeyMessageSignals(fset, rel, file)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no Go sources checked")
	}
	slices.Sort(signals)
	for _, signal := range signals {
		t.Error(signal)
	}
}

func TestAddressKeyMessageGuardRecognizesMultilineCalls(t *testing.T) {
	source := `package p
func bad(a objectmodel.Address) error {
	return kernel.Fail(kernel.ErrInvalid,
		"address %s is broken",
		objectmodel.AddressKey(a))
}
func badSprintf(a objectmodel.Address) string {
	return fmt.Sprintf("at %s", AddressKey(a))
}
func badErrorf(a objectmodel.Address) error {
	return fmt.Errorf("at %s: %w", strings.ToUpper(objectmodel.AddressKey(a)), nil)
}
func badLocal(a objectmodel.Address) error {
	where := objectmodel.AddressKey(a)
	where += " at"
	return kernel.Fail(kernel.ErrInvalid, "declaration %s", where)
}
func badFieldKey(f FieldRef) error {
	return kernel.Fail(kernel.ErrInvalid, "field %s", f.Key())
}
func good(a objectmodel.Address) (map[string]bool, error) {
	keys := map[string]bool{objectmodel.AddressKey(a): true}
	return keys, kernel.Fail(kernel.ErrInvalid, "address %s is broken", objectmodel.AddressLabel(a))
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	signals := addressKeyMessageSignals(fset, "fixture.go", file)
	if len(signals) != 5 {
		t.Fatalf("signals = %v, want exactly the five message calls", signals)
	}
	for _, want := range []string{"Fail", "Sprintf", "Errorf"} {
		if !slices.ContainsFunc(signals, func(signal string) bool { return strings.Contains(signal, " "+want+" ") }) {
			t.Fatalf("signals = %v, missing %s", signals, want)
		}
	}
}

func TestAddressKeyMessageGuardResolvesLocalScopes(t *testing.T) {
	for _, test := range []struct {
		name     string
		receiver string
		body     string
		want     int
	}{
		{
			name: "clean inner variable shadows key",
			body: `where := AddressKey(a)
				{ where := "display"; _ = fmt.Sprintf("%s", where) }
				return fmt.Errorf("%s", where)`,
			want: 1,
		},
		{
			name: "key in inner scope leaves outer variable clean",
			body: `where := "display"
				{ where := AddressKey(a); _ = fmt.Sprintf("%s", where) }
				return fmt.Errorf("%s", where)`,
			want: 1,
		},
		{
			name: "closure captures key",
			body: `where := AddressKey(a)
				return func() error { return fmt.Errorf("%s", where) }()`,
			want: 1,
		},
		{
			name: "closure parameter shadows key",
			body: `where := AddressKey(a)
				_ = where
				return func(where string) error { return fmt.Errorf("%s", where) }("display")`,
			want: 0,
		},
		{
			name: "assignment resolves existing local",
			body: `where := ""
				where = AddressKey(a)
				return fmt.Errorf("%s", where)`,
			want: 1,
		},
		{
			name:     "receiver type is declared in another file",
			receiver: "(store *Store) ",
			body: `where := AddressKey(a)
				return fmt.Errorf("%s", where)`,
			want: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fset := token.NewFileSet()
			source := "package p\nfunc " + test.receiver + "example(a objectmodel.Address) error {\n" + test.body + "\n}"
			file, err := parser.ParseFile(fset, "fixture.go", source, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			signals := addressKeyMessageSignals(fset, "fixture.go", file)
			if len(signals) != test.want {
				t.Fatalf("signals = %v, want %d message calls", signals, test.want)
			}
		})
	}
}

var addressMessageCalls = map[string]bool{"Fail": true, "Sprintf": true, "Errorf": true}

func addressKeyMessageSignals(fset *token.FileSet, rel string, file *ast.File) []string {
	bindings := &types.Info{
		Defs: map[*ast.Ident]types.Object{},
		Uses: map[*ast.Ident]types.Object{},
	}
	// Files are inspected independently, including fixtures with unresolved
	// imports and types. Keep resolving lexical bindings after these errors;
	// the guard needs variable identity, not valid expression types.
	config := types.Config{Error: func(error) {}}
	_, _ = config.Check(file.Name.Name, fset, []*ast.File{file}, bindings)
	var signals []string
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		keys := addressKeyLocals(fn.Body, bindings)
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !addressMessageCalls[calledName(call.Fun)] {
				return true
			}
			for _, arg := range call.Args {
				ast.Inspect(arg, func(inner ast.Node) bool {
					if usesAddressKey(inner, keys, bindings) {
						pos := fset.Position(inner.Pos())
						signals = append(signals, rel+":"+strconv.Itoa(pos.Line)+": "+calledName(call.Fun)+" message uses an internal key; name addresses with AddressLabel and fields with their display form")
						return false
					}
					return true
				})
			}
			return false
		})
	}
	return signals
}

// addressKeyLocals names locals assigned from an expression that calls
// AddressKey, so a key routed through a variable still counts as a leak.
func addressKeyLocals(body *ast.BlockStmt, bindings *types.Info) map[types.Object]bool {
	keys := map[types.Object]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, rhs := range assign.Rhs {
			id, ok := assign.Lhs[i].(*ast.Ident)
			if !ok {
				continue
			}
			object := bindings.ObjectOf(id)
			if object == nil {
				continue
			}
			ast.Inspect(rhs, func(inner ast.Node) bool {
				if call, ok := inner.(*ast.CallExpr); ok && isInternalKeyCall(call) {
					keys[object] = true
				}
				return true
			})
		}
		return true
	})
	return keys
}

func usesAddressKey(node ast.Node, keys map[types.Object]bool, bindings *types.Info) bool {
	switch n := node.(type) {
	case *ast.CallExpr:
		return isInternalKeyCall(n)
	case *ast.Ident:
		object := bindings.ObjectOf(n)
		return object != nil && keys[object]
	}
	return false
}

// isInternalKeyCall matches AddressKey and zero-argument Key methods: both
// join identity parts with a control separator for map lookups.
func isInternalKeyCall(call *ast.CallExpr) bool {
	if calledName(call.Fun) == "AddressKey" {
		return true
	}
	_, method := call.Fun.(*ast.SelectorExpr)
	return method && calledName(call.Fun) == "Key" && len(call.Args) == 0
}

func calledName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}
