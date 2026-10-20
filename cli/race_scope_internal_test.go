package cli

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const raceScopeFile = "../scripts/race-cli-tests.txt"

// TestRaceScopeCoversCLIConcurrencyTests keeps `make test-race` honest for kc/cli: the
// package runs under -race only for the listed tests, so every test that exercises
// concurrency in its own body must be listed, and every listed name must still exist.
func TestRaceScopeCoversCLIConcurrencyTests(t *testing.T) {
	listed := readRaceScope(t, raceScopeFile)
	declared, concurrent := scanCLITests(t, ".")

	var missing, stale []string
	for name := range concurrent {
		if !listed[name] {
			missing = append(missing, name+" ("+concurrent[name]+")")
		}
	}
	for name := range listed {
		if !declared[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("concurrent cli tests missing from %s:\n  %s", raceScopeFile, strings.Join(missing, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("%s lists undeclared cli tests:\n  %s", raceScopeFile, strings.Join(stale, "\n  "))
	}
}

func TestRaceScopeScannerRecognisesConcurrencySignals(t *testing.T) {
	dir := t.TempDir()
	src := `package cli

import (
	"sync"
	"testing"
)

func TestGoroutine(t *testing.T) { go func() {}() }
func TestParallel(t *testing.T) { t.Parallel() }
func TestWaitGroup(t *testing.T) { var wg sync.WaitGroup; wg.Wait() }
func TestNamedConcurrentWriters(t *testing.T) {}
func TestSerial(t *testing.T) { _ = t.Name() }
func helper(t *testing.T) { go func() {}() }
`
	if err := os.WriteFile(filepath.Join(dir, "sample_test.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	declared, concurrent := scanCLITests(t, dir)
	want := map[string]string{
		"TestGoroutine":              "go statement",
		"TestParallel":               "t.Parallel",
		"TestWaitGroup":              "sync.WaitGroup",
		"TestNamedConcurrentWriters": "name",
	}
	if len(concurrent) != len(want) {
		t.Fatalf("concurrent = %v, want %v", concurrent, want)
	}
	for name, signal := range want {
		if concurrent[name] != signal {
			t.Fatalf("concurrent[%s] = %q, want %q", name, concurrent[name], signal)
		}
	}
	if !declared["TestSerial"] || declared["helper"] {
		t.Fatalf("declared = %v", declared)
	}
}

func readRaceScope(t *testing.T, path string) map[string]bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	names := map[string]bool{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if names[line] {
			t.Errorf("%s lists %s twice", path, line)
		}
		names[line] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

func scanCLITests(t *testing.T, dir string) (declared map[string]bool, concurrent map[string]string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	declared = map[string]bool{}
	concurrent = map[string]string{}
	fset := token.NewFileSet()
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil || !isTopLevelTest(fn) {
				continue
			}
			declared[fn.Name.Name] = true
			if signal := concurrencySignal(fn); signal != "" {
				concurrent[fn.Name.Name] = signal
			}
		}
	}
	return declared, concurrent
}

func isTopLevelTest(fn *ast.FuncDecl) bool {
	name := fn.Name.Name
	if !strings.HasPrefix(name, "Test") || name == "TestMain" {
		return false
	}
	params := fn.Type.Params.List
	if len(params) != 1 {
		return false
	}
	star, ok := params[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "T"
}

func concurrencySignal(fn *ast.FuncDecl) string {
	signal := ""
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if signal != "" {
			return false
		}
		switch n := node.(type) {
		case *ast.GoStmt:
			signal = "go statement"
		case *ast.CallExpr:
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Parallel" && len(n.Args) == 0 {
				signal = "t.Parallel"
			}
		case *ast.SelectorExpr:
			if pkg, ok := n.X.(*ast.Ident); ok {
				switch {
				case pkg.Name == "sync" && n.Sel.Name == "WaitGroup":
					signal = "sync.WaitGroup"
				case pkg.Name == "errgroup":
					signal = "errgroup"
				}
			}
		}
		return true
	})
	if signal == "" && strings.Contains(fn.Name.Name, "Concurrent") {
		signal = "name"
	}
	return signal
}
