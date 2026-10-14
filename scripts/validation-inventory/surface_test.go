package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSurfaceCheckerUsesBothProductionHTTPRouteOwners(t *testing.T) {
	for _, tc := range []struct {
		name    string
		file    string
		content string
		failure string
	}{
		{name: "composedOwners"},
		{"missingModuleNamespace", "snapshot/service/file_http.go", "mux.HandleFunc(\"GET /admin/v1/grants\", handler)\n", "missing /snapshot/v1/files route namespace"},
		{"moduleCLIRegistry", "snapshot/service/http.go", "cliSurface\n", "typed HTTP routes depend on CLI registries/parser"},
		{"moduleUntypedInvocation", "snapshot/service/http.go", "client.Invoke()\n", "untyped client invocation"},
		{"moduleLegacyHTTP", "snapshot/service/http.go", "mux.Handle(\"POST /v1/old\", handler)\n", "legacy verb HTTP implementation"},
		{"serverCLIRegistry", "server/service_routes.go", "ParseArgs\n", "typed HTTP routes depend on CLI registries/parser"},
		{"retiredAgentTool", "cli/tools.go", "knowledge_read\n", "retired model tools"},
		{"retiredSnapshotAgentTool", "snapshot/cli/tools.go", "knowledge_read\n", "retired model tools"},
		{"untypedSnapshotSDK", "snapshot/client/client.go", "client.Invoke()\n", "untyped client invocation"},
		{"staleSnapshotEntryExample", "snapshot/cli/README.md", "kc define-workspace old\n", "stale flat CLI examples"},
		{"staleSnapshotCLIExample", "snapshot/dataset/README.md", "kc define-workspace old\n", "stale flat CLI examples"},
		{"retiredModuleCatalogExample", "snapshot/service/README.md", "go run ./cmd/kc -- catalog use old\n", "retired Catalog CLI examples"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := surfaceFixture(t)
			if tc.file != "" {
				if err := os.WriteFile(filepath.Join(root, tc.file), []byte(tc.content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("bash", filepath.Join(root, "scripts", "check-surface.sh"))
			output, err := cmd.CombinedOutput()
			if tc.failure == "" {
				if err != nil {
					t.Fatalf("composed production owners rejected: %v\n%s", err, output)
				}
			} else if err == nil || !strings.Contains(string(output), tc.failure) {
				t.Fatalf("surface violation escaped its original guard: err=%v, want=%s\n%s", err, tc.failure, output)
			}
		})
	}
}

func surfaceFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	script, err := os.ReadFile(filepath.Join("..", "check-surface.sh"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"scripts/check-surface.sh":   string(script),
		"Makefile":                   "",
		".dockerignore":              "",
		".github/fixture.yml":        "",
		"cmd/fixture.go":             "package main\n",
		"README.md":                  "",
		"doc/fixture.md":             "",
		"cli/README.md":              "",
		"client/client.go":           "package client\n",
		"snapshot/client/client.go":  "package client\n",
		"snapshot/client/README.md":  "",
		"snapshot/cli/commands.go":   "package cli\n",
		"snapshot/cli/README.md":     "",
		"liveaccess/README.md":       "",
		"object/identity.go":         "package object\n",
		"snapshot/README.md":         "",
		"snapshot/dataset/README.md": "",
		"snapshot/access/README.md":  "",
		"snapshot/service/README.md": "",
		"snapshot/console/README.md": "",
		"server/service_routes.go": "mux.HandleFunc(\"GET /snapshot/v1/inventory\", handler)\n" +
			"mux.HandleFunc(\"POST /knowledge/v1/read\", handler)\n" +
			"mux.HandleFunc(\"POST /writer/v1/commits\", handler)\n" +
			"mux.HandleFunc(\"POST /governance/v1/proposals\", handler)\n" +
			"mux.HandleFunc(\"POST /identity/v1/token\", handler)\n" +
			"mux.HandleFunc(\"POST /operations/v1/hooks\", handler)\n",
		"snapshot/service/file_http.go": "mux.HandleFunc(\"POST /snapshot/v1/files/file:read\", handler)\n" +
			"mux.HandleFunc(\"GET /admin/v1/grants\", handler)\n",
		"snapshot/service/http.go": "mux.Handle(\"GET /ui/\", handler)\n",
	}
	for path, content := range files {
		absolute := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
