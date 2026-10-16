package arch_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// Automatic fixtures share one daemon budget even when separate frozen checkouts
// run the suite. Exercise the process boundary with fake Docker, never services.
func TestAutomaticOpenSearchFixtureLifecycleContracts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	root := moduleRoot(t)
	cmd := exec.CommandContext(ctx, "python3", filepath.Join(root, "scripts", "opensearch_auto_test.py"), "-v")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("automatic fixture lifecycle contracts: %v\n%s", err, output)
	}
	if !regexp.MustCompile(`Ran [1-9][0-9]* tests?`).Match(output) {
		t.Fatalf("automatic fixture lifecycle suite ran no tests:\n%s", output)
	}
}
