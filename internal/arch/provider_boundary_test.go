package arch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Knowledge-provider implementations live outside the protocol module. Local
// black-box fixtures may be kept below ignored .data while they are incubating.
func TestProviderImplementationRootsStayOutOfMain(t *testing.T) {
	root := moduleRoot(t)
	for _, rel := range []string{".scenes", "collectors", "scenario", "validation", filepath.Join("tests", "collectors")} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			t.Errorf("provider implementation %s must not live in main", rel)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

func TestRuntimeHasNoBundledDataWarehouseStory(t *testing.T) {
	root := moduleRoot(t)
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

func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
