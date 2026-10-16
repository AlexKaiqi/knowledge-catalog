package cli_test

import (
	"os"
	"testing"

	"kc/internal/testkit"
	"kc/invocation"
)

func TestSceneRetiredLocalCommandsUsePublicBoundary(t *testing.T) {
	t.Setenv("KC_TEST_OPENSEARCH_URL", "")
	home := testkit.TempDir(t)
	body(t, kc(home, "local", "init"))
	t.Setenv("KC_HOME", home)
	world := newSceneWorldAt(t, testkit.TempDir(t), nil)
	defer world.close()
	world.runCommand("kc local grant bootstrap --principal agent:other", "")
	world.thenError("USAGE_INVALID")
	if _, err := os.Stat(invocation.AllowPath(home)); !os.IsNotExist(err) {
		t.Fatalf("retired scene command wrote to ambient fixture home: %v", err)
	}
}
