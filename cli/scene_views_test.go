package cli_test

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// The Python projection owns view selection; run its counterexamples and the
// actual catalog's union audit as part of the normal scene contract suite.
func TestSceneViews(t *testing.T) {
	for _, check := range []struct {
		name string
		args []string
	}{
		{"projection_contract", []string{"-m", "unittest", "discover", "-s", filepath.Join(scenesRoot(), "_tests"), "-p", "test_views.py"}},
		{"product_contract", []string{"-m", "unittest", "discover", "-s", filepath.Join(scenesRoot(), "_tests"), "-p", "test_product_views.py"}},
		{"fixture_contract", []string{"-m", "unittest", "discover", "-s", filepath.Join(scenesRoot(), "_tests"), "-p", "test_state_fixtures.py"}},
		{"walkthrough_probe", []string{"-m", "unittest", "discover", "-s", filepath.Join(scenesRoot(), "_tests"), "-p", "test_goto_probe.py"}},
		{"catalog_union", []string{filepath.Join(scenesRoot(), "tree.py"), "--check-views"}},
		{"product_links", []string{filepath.Join(scenesRoot(), "tree.py"), "--check-product"}},
		{"reused_fixtures", []string{filepath.Join(scenesRoot(), "tree.py"), "--check-states"}},
	} {
		t.Run(check.name, func(t *testing.T) {
			out, err := exec.Command("python3", check.args...).CombinedOutput()
			if err != nil {
				t.Fatalf("scene views: %v\n%s", err, out)
			}
		})
	}
}
