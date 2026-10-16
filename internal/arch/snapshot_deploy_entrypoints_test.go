package arch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

func TestLakeFSDeploymentRecoversFailureWithoutReplayingBootstrap(t *testing.T) {
	for _, tc := range []struct {
		file, service, restart string
	}{
		{"compose.yaml", "lakefs", "on-failure:3"},
		{"compose.yaml", "bootstrap", "no"},
		{"compose.dev.yaml", "lakefs", "unless-stopped"},
	} {
		t.Run(tc.file+"/"+tc.service, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(moduleRoot(t), "scripts/deploy", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			var config struct {
				Services map[string]struct {
					Restart string `yaml:"restart"`
				} `yaml:"services"`
			}
			if err := yaml.Unmarshal(raw, &config); err != nil {
				t.Fatal(err)
			}
			if got := config.Services[tc.service].Restart; got != tc.restart {
				t.Fatalf("%s restart policy = %q, want %q", tc.service, got, tc.restart)
			}
		})
	}
}

func TestLakeFSDeploymentUsesMeasuredGoMemoryBudget(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(moduleRoot(t), "scripts/deploy/compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
			MemoryLimit string            `yaml:"mem_limit"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	lakefs := config.Services["lakefs"]
	if got := lakefs.Environment["GOMEMLIMIT"]; got != "1536MiB" {
		t.Errorf("lakeFS Go memory budget = %q, want measured 1536MiB budget", got)
	}
	if got := lakefs.Environment["LAKEFS_COMMITTED_SSTABLE_MEMORY_CACHE_SIZE_BYTES"]; got != "67108864" {
		t.Errorf("lakeFS SSTable memory cache = %q, want bounded 64MiB cache", got)
	}
	if lakefs.MemoryLimit != "" || lakefs.Environment["GOGC"] == "off" {
		t.Error("soft runtime budget must retain ordinary GC without adding a kernel memory limit")
	}
}
