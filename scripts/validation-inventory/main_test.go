package main

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestInventoryCountsDeclarationsWithoutClaimingExecution(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture_test.go", `package fixture
import "testing"
// func TestComment(t *testing.T) {}
func TestMain(m *testing.M) {}
func TestReal(t *testing.T) { t.Run("dynamic", func(t *testing.T) {}) }
func BenchmarkRead(b *testing.B) {}
func FuzzRead(f *testing.F) {}
func helper() {}
func TestNamedHelper(s string) {}
func TestWrongType(t *testing.B) {}
func TestGeneric[T any](t *testing.T) {}
`, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	got := testDeclarations(fset, file, "fixture_test.go")
	if len(got) != 3 || got[0].Name != "TestReal" || got[0].Line != 5 || got[0].Kind != "test" {
		t.Fatalf("declared tests include comment, TestMain, or runtime subtest: %#v", got)
	}
}

func TestInventoryRejectsDynamicHTTPRegistration(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "routes.go", `package fixture
func register() { mux.HandleFunc(pattern, handler) }
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := routeDeclarations(fset, file, "routes.go"); err == nil {
		t.Fatal("dynamic registration was silently omitted from denominator")
	}
}

func TestInventoryReadsOnlyNamedPublicRegistry(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "surface.go", `package fixture
var unrelated = map[string]string{"fake": "action"}
var cliSurface = map[string]commandSurface{"knowledge read": {"knowledge-read", "knowledge.read"}}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := cliDeclarations(fset, file, "surface.go")
	if err != nil || len(got) != 1 || got[0].Command != "knowledge read" || got[0].Action != "knowledge.read" {
		t.Fatalf("public denominator: %#v, %v", got, err)
	}
}
