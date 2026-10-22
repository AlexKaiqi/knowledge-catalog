package arch_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Authentication is a replaceable capability owned by identity/ (AUTH-LAYOUT-01).
// Concrete authenticators live in identity/<name>; production code elsewhere
// consumes identity ports and never selects or branches on a provider by name.

// authenticatorAssemblyRoots are the only production files allowed to link
// concrete authenticator packages: the builtin set for kc serve and deployment
// validation, and the Server's default self-assertion seam (--auth local).
// Deployment validation lives in home/config, so that file is the root that
// must link the builtin set for the names Validate accepts.
var authenticatorAssemblyRoots = []string{"server/serve_handler.go", "server/serve_auth.go", "home/config/deployment.go"}

// authenticatorNameScopes must not compare against provider names; they hold
// an identity.HTTPAuthenticator and ask it, not its label.
var authenticatorNameScopes = []string{"server", "snapshot/service"}

// retiredAuthenticatorNames stay forbidden as branch literals after removal.
// "lakefs" is deliberately absent: it remains the live storage driver name, and
// the retired --auth lakefs cannot return without a provider under identity/.
var retiredAuthenticatorNames = []string{"gitea"}

func TestAuthenticatorsAreConfinedToIdentity(t *testing.T) {
	files := productionSourceFiles(t, nil)
	names := authenticatorProviderNames(t)
	if signals := authenticatorSignals(files, names); len(signals) != 0 {
		t.Fatalf("authentication leaked out of identity/:\n%s", strings.Join(signals, "\n"))
	}
}

// Test doubles come from identity/identitytest; a package test declaring its
// own Authenticate method is a copied double.
func TestAuthenticatorDoublesComeFromIdentitytest(t *testing.T) {
	root := moduleRoot(t)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" || rel == "identity" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, signal := range authenticatorImplementations(rel, file) {
			t.Errorf("%s; use kc/identity/identitytest", signal)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticatorGuardRejectsLeaks(t *testing.T) {
	names := []string{"taihu", "gitea"}
	base := map[string]*ast.File{
		"identity/taihu/authenticator.go": declarativeIndexGuardFixture(t, "a.go", `package taihu; import ("context"; "net/http"); type Authenticator struct{}; func (Authenticator) Authenticate(context.Context, http.Header) (any, error) { return nil, nil }`),
		"server/serve_handler.go":         declarativeIndexGuardFixture(t, "s.go", `package server; import _ "kc/identity/builtin"`),
		"server/serve_auth.go":            declarativeIndexGuardFixture(t, "o.go", `package server; func f(mode string) bool { return mode == "local" }`),
	}
	if signals := authenticatorSignals(base, names); len(signals) != 0 {
		t.Fatalf("correct identity layout rejected: %v", signals)
	}
	for _, fixture := range []struct{ name, path, source, want string }{
		{"implementation-outside", "server/auth_x.go", `package server; import ("context"; "net/http"); type x struct{}; func (*x) Authenticate(_ context.Context, h http.Header) (any, error) { return nil, nil }`, "implements Authenticate"},
		{"name-comparison", "server/snapshot_x.go", `package server; func f(mode string) bool { return mode == "taihu" }`, `compares against authenticator name "taihu"`},
		{"retired-name-switch", "snapshot/service/x.go", `package service; func f(m string) { switch m { case "gitea": } }`, `compares against authenticator name "gitea"`},
		{"provider-import", "cli/x.go", `package cli; import "kc/identity/taihu"; var _ = taihu.Name`, "imports concrete authenticator kc/identity/taihu"},
		{"builtin-import", "invocation/x.go", `package invocation; import _ "kc/identity/builtin"`, "imports concrete authenticator kc/identity/builtin"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			files := maps.Clone(base)
			files[fixture.path] = declarativeIndexGuardFixture(t, fixture.path, fixture.source)
			signals := authenticatorSignals(files, names)
			if !slices.ContainsFunc(signals, func(signal string) bool {
				return strings.Contains(signal, fixture.path) && strings.Contains(signal, fixture.want)
			}) {
				t.Fatalf("authentication leak was not identified: %v", signals)
			}
		})
	}
}

// authenticatorProviderNames are the concrete authenticator directories under
// identity/ (excluding the builtin linker, the identitytest doubles, and the self-assertion seam "local",
// which is the default mode rather than a selectable provider), plus retired names.
func authenticatorProviderNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(moduleRoot(t), "identity"))
	if err != nil {
		t.Fatal(err)
	}
	names := slices.Clone(retiredAuthenticatorNames)
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "builtin" && entry.Name() != "local" && entry.Name() != "identitytest" {
			names = append(names, entry.Name())
		}
	}
	if !slices.Contains(names, "taihu") {
		t.Fatalf("identity/taihu not found; provider names %v", names)
	}
	return names
}

func authenticatorSignals(files map[string]*ast.File, names []string) []string {
	var signals []string
	paths := slices.Collect(maps.Keys(files))
	sort.Strings(paths)
	for _, path := range paths {
		file := files[path]
		inIdentity := strings.HasPrefix(path, "identity/")
		if !inIdentity {
			signals = append(signals, authenticatorImplementations(path, file)...)
			signals = append(signals, authenticatorImports(path, file)...)
		}
		if inScope(path, authenticatorNameScopes) {
			signals = append(signals, authenticatorNameComparisons(path, file, names)...)
		}
	}
	return signals
}

func inScope(path string, scopes []string) bool {
	return slices.ContainsFunc(scopes, func(scope string) bool { return strings.HasPrefix(path, scope+"/") })
}

func authenticatorImplementations(path string, file *ast.File) []string {
	var signals []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name.Name != "Authenticate" || !takesHTTPHeader(fn.Type.Params) {
			continue
		}
		signals = append(signals, fmt.Sprintf("%s implements Authenticate(context.Context, http.Header); authenticators live under identity/", path))
	}
	return signals
}

func takesHTTPHeader(params *ast.FieldList) bool {
	if params == nil {
		return false
	}
	for _, field := range params.List {
		if sel, ok := field.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "Header" {
			return true
		}
	}
	return false
}

func authenticatorImports(path string, file *ast.File) []string {
	var signals []string
	for _, spec := range file.Imports {
		target, err := strconv.Unquote(spec.Path.Value)
		if err != nil || !strings.HasPrefix(target, "kc/identity/") || slices.Contains(authenticatorAssemblyRoots, path) {
			continue
		}
		signals = append(signals, fmt.Sprintf("%s imports concrete authenticator %s; only %v link authenticators", path, target, authenticatorAssemblyRoots))
	}
	return signals
}

func authenticatorNameComparisons(path string, file *ast.File, names []string) []string {
	var signals []string
	report := func(expr ast.Expr) {
		lit, ok := expr.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return
		}
		value, err := strconv.Unquote(lit.Value)
		if err == nil && slices.Contains(names, strings.ToLower(value)) {
			signals = append(signals, fmt.Sprintf("%s compares against authenticator name %q; ask the identity.HTTPAuthenticator instead", path, value))
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.BinaryExpr:
			if n.Op == token.EQL || n.Op == token.NEQ {
				report(n.X)
				report(n.Y)
			}
		case *ast.CaseClause:
			for _, expr := range n.List {
				report(expr)
			}
		}
		return true
	})
	return signals
}
