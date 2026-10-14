package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingServerGuidesConsumersToExistingServiceAndLogin(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"KC_SERVER_URL", "KC_AUTH_TOKEN", "KC_AS", "KC_HOME", "KC_DATASET"} {
		t.Setenv(name, "")
	}
	t.Setenv("KC_CONFIG_DIR", filepath.Join(dir, "client"))
	for _, argv := range [][]string{{"show"}, {"files", "list", "--dataset", "delivery"}, {"dataset", "show", "delivery"}} {
		result := Run(argv)
		if result.Status != 1 {
			t.Fatalf("missing service unexpectedly executed: %q %#v", argv, result)
		}
		for _, claim := range []string{"USAGE_INVALID", "requires KC Server", "existing service address", "--server", "KC_SERVER_URL", "kc help login", "deployment owners"} {
			if !strings.Contains(result.Stdout, claim) {
				t.Errorf("%q missing service guidance %q: %s", argv, claim, result.Stdout)
			}
		}
		if strings.Contains(result.Stdout, "use kc deployment init") {
			t.Errorf("ordinary consumer was directed to initialize a deployment: %q", argv)
		}
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("missing server guidance created local deployment/session: %v %v", entries, err)
	}
}
