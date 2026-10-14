package arch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotDeploymentHarnessUsesSelectedRegistrationEntrypoints(t *testing.T) {
	root := moduleRoot(t)
	for _, name := range []string{"bootstrap.sh", "deploy.sh", "compose.yaml", "cli.sh", "cli.profile.sh", "README.md"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, "scripts/deploy", name))
			if err != nil {
				t.Fatal(err)
			}
			for _, retired := range []string{"--catalog", "KC_CATALOG", "kc catalog list"} {
				if strings.Contains(string(raw), retired) {
					t.Errorf("deployment entrypoint retains retired registration surface %q", retired)
				}
			}
		})
	}
	bootstrap, err := os.ReadFile(filepath.Join(root, "scripts/deploy/bootstrap.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--registry-id", "--registry-driver", "--registry-dsn"} {
		if !strings.Contains(string(bootstrap), flag) {
			t.Errorf("bootstrap does not bind the selected registry coordinate %s", flag)
		}
	}
}
