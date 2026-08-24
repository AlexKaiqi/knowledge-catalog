package arch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Scene implementations are checked out below .scenes and must not become
// packages of the protocol module. Business fixtures may still appear in
// *_test.go and test scripts; they are not shipped by the reference runtime.
func TestSceneImplementationRootsStayOutOfMain(t *testing.T) {
	root := moduleRoot(t)
	if isDataWarehouseScene(root) {
		t.Skip("main boundary check does not apply to the data warehouse scene")
	}
	for _, rel := range []string{"collectors", "scenario", filepath.Join("tests", "collectors")} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			t.Errorf("scene implementation %s must not live in main", rel)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

func TestRuntimeHasNoBundledDataWarehouseStory(t *testing.T) {
	root := moduleRoot(t)
	if isDataWarehouseScene(root) {
		t.Skip("main boundary check does not apply to the data warehouse scene")
	}
	markers := []string{
		"Table:dwd.trade_order",
		"Metric:gmv",
		"GMV 不含 7 日内退货",
		"schema/table.structure",
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) || d.Name() == "testdata" || d.Name() == "test" || d.Name() == "tests" || d.Name() == "validation" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		ext := filepath.Ext(d.Name())
		if ext != ".go" && ext != ".html" && ext != ".ts" && ext != ".js" {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, marker := range markers {
			if strings.Contains(string(body), marker) {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("business fixture %q is bundled in runtime file %s", marker, rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func isDataWarehouseScene(root string) bool {
	_, err := os.Stat(filepath.Join(root, "validation", "fixtures", "tpch-sf001"))
	return err == nil
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
