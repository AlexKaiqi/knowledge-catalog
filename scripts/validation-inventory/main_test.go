package main

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
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
	for _, method := range []string{"HandleFunc", "Handle"} {
		t.Run(method, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "routes.go", "package fixture\nfunc register() { mux."+method+"(pattern, handler) }\n", 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := routeDeclarations(fset, file, "routes.go"); err == nil {
				t.Fatal("dynamic registration was silently omitted from denominator")
			}
		})
	}
}

func TestInventoryIncludesModuleAndServerProductionRoutes(t *testing.T) {
	root := filepath.Join("..", "..")
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	cmd.Dir = root
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var routes []routeEntry
	owners := map[string]int{}
	for _, path := range strings.Split(string(raw), "\x00") {
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || !productionHTTPSource(path) {
			continue
		}
		source, err := os.ReadFile(filepath.Join(root, path))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, path, source, 0)
		if err != nil {
			t.Fatal(err)
		}
		found, err := routeDeclarations(fset, file, path)
		if err != nil {
			t.Fatal(err)
		}
		for _, route := range found {
			owner := "server"
			if strings.HasPrefix(route.File, "snapshot/service/") {
				owner = "snapshot/service"
			}
			owners[owner]++
			lines := strings.Split(string(source), "\n")
			if route.File != path || route.Line < 1 || route.Line > len(lines) || !strings.Contains(lines[route.Line-1], `"`+route.Pattern+`"`) {
				t.Fatalf("production route lost its source location: %#v", route)
			}
		}
		routes = append(routes, found...)
	}
	// The closed set is 90 routes: the four indexes:* operations replace the
	// two retired projections:* operations, so Server owns two more than before.
	if owners["server"] != 61 || owners["snapshot/service"] != 29 || len(routes) != 90 {
		t.Fatalf("production routes omitted or assigned to stale owners: counts=%v, total=%d", owners, len(routes))
	}
	patterns, err := goldenRoutesFrom(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRouteInventory(routes, patterns); err != nil {
		t.Fatal(err)
	}
	for _, route := range routes {
		if route.Pattern == "POST /operations/v1/projections:notice" {
			t.Fatalf("retired public projection notice remains in the inventory: %#v", route)
		}
		if (route.Pattern == "GET /ui/" || route.Pattern == "GET /identity/v1/whoami" || route.Pattern == "POST /snapshot/v1/files/file:read" || route.Pattern == "POST /snapshot/v1/datasets" || route.Pattern == "PUT /admin/v1/grants/{grant}" || route.Pattern == "GET /snapshot/v1/inventory" || route.Pattern == "GET /snapshot/v1/audit" || route.Pattern == "POST /snapshot/v1/repositories" || route.Pattern == "DELETE /snapshot/v1/repositories/{repository}") && !strings.HasPrefix(route.File, "snapshot/service/") {
			t.Fatalf("Snapshot Store route still points to an old Server owner: %#v", route)
		}
	}
}

func TestInventoryRejectsRouteClosureAndDuplicates(t *testing.T) {
	patterns := []string{"GET /one", "GET /two"}
	for _, tc := range []struct {
		name   string
		routes []routeEntry
	}{
		{"missing", []routeEntry{{Pattern: "GET /one"}}},
		{"extra", []routeEntry{{Pattern: "GET /one"}, {Pattern: "GET /two"}, {Pattern: "GET /three"}}},
		{"substituted", []routeEntry{{Pattern: "GET /one"}, {Pattern: "GET /other"}}},
		{"duplicateAcrossOwners", []routeEntry{{"GET /one", "server/routes.go", 1}, {"GET /one", "snapshot/service/routes.go", 2}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRouteInventory(tc.routes, patterns)
			if err == nil {
				t.Fatal("invalid HTTP inventory matched the closed registry")
			}
			if tc.name == "duplicateAcrossOwners" && (!strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "server/routes.go:1") || !strings.Contains(err.Error(), "snapshot/service/routes.go:2")) {
				t.Fatalf("duplicate must identify both production owners: %v", err)
			}
		})
	}
	if err := validateRouteInventory([]routeEntry{{Pattern: "GET /two"}, {Pattern: "GET /one"}}, patterns); err != nil {
		t.Fatalf("exact production closure rejected: %v", err)
	}
}

func TestInventoryReadsOnlyNamedPublicRegistry(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "surface.go", `package fixture
var unrelated = map[string]string{"fake": "action"}
var cliSurface = map[string]commandSurface{"read": {"object-read", "object.read"}}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := cliDeclarations(fset, file, "surface.go")
	if err != nil || len(got) != 1 || got[0].Command != "read" || got[0].Action != "object.read" {
		t.Fatalf("public denominator: %#v, %v", got, err)
	}
}

func TestInventoryChecksEvidenceTablesThatExist(t *testing.T) {
	documents := validationEvidenceDocuments()
	for _, required := range []string{"doc/engineering/test-catalog.md", "doc/engineering/architecture-invariants.md"} {
		if !slices.Contains(documents, required) {
			t.Errorf("%s Test references are outside the checked evidence inventory", required)
		}
	}
	for _, document := range documents {
		if _, err := os.Stat(filepath.Join("..", "..", document)); err != nil {
			t.Errorf("evidence document %s is not readable: %v", document, err)
		}
	}
}
