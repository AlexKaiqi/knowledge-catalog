package cli

import (
	"encoding/base64"
	"testing"

	"kc/internal/testkit"
)

func TestRelativeMountPath(t *testing.T) {
	for _, tc := range []struct {
		mount string
		path  string
		want  string
		ok    bool
	}{
		{"knowledge/team", "knowledge/team/README.md", "README.md", true},
		{"knowledge/team", "knowledge/team", "", true},
		{"knowledge/team", "knowledge/other/a", "", false},
		{"", "README.md", "README.md", true},
	} {
		got, ok := relativeMountPath(tc.mount, tc.path)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("relativeMountPath(%q, %q) = %q, %v; want %q, %v", tc.mount, tc.path, got, ok, tc.want, tc.ok)
		}
	}
}

func TestPrepareWorkspaceFSMakesOneTargetPerRecipePath(t *testing.T) {
	home := testkit.TempDir(t)
	project := testkit.TempDir(t)
	one := "kr://acme/team/docs"
	two := "kr://acme/org/policy"
	mustWorkspaceFSRun(t, home, "init", "--catalog", "kr://acme/catalog")
	mustWorkspaceFSRun(t, home, "repo-add", "--repo", one)
	mustWorkspaceFSRun(t, home, "repo-add", "--repo", two)
	mustWorkspaceFSRun(t, home, "define-workspace", "--workspace", "agent", "--revision", "1",
		"--source", one+"=refs/heads/main@docs/team@team",
		"--source", one+"=refs/heads/main@docs/runbooks@runbooks",
		"--source", two+"=refs/heads/main@knowledge/policy")
	mustWorkspaceFSRun(t, home, "vfs-write", "--workspace", "agent", "--command-id", "team-file",
		"--path", "docs/team/README.md", "--content", base64.StdEncoding.EncodeToString([]byte("team\n")))
	mustWorkspaceFSRun(t, home, "vfs-write", "--workspace", "agent", "--command-id", "policy-file",
		"--path", "knowledge/policy/rules.md", "--content", base64.StdEncoding.EncodeToString([]byte("policy\n")))
	mustWorkspaceFSRun(t, home, "vfs-write", "--workspace", "agent", "--command-id", "runbook-file",
		"--path", "docs/runbooks/incident.md", "--content", base64.StdEncoding.EncodeToString([]byte("incident\n")))

	plan, manifest, closeView, err := prepareWorkspaceFS(workspaceFSConfig{
		home: home, workspace: "agent", root: project,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeView()
	if len(plan.Mounts) != 3 || len(manifest.Mounts) != 3 {
		t.Fatalf("plan=%#v manifest=%#v", plan.Mounts, manifest.Mounts)
	}
	if plan.PinID == "" || plan.PinID != manifest.PinID {
		t.Fatalf("pin mismatch: %#v %#v", plan.PinID, manifest.PinID)
	}
	if len(plan.Mounts[0].Files) != 1 {
		t.Fatalf("first mount files: %#v", plan.Mounts[0].Files)
	}
	content, err := plan.Mounts[0].Files[0].Read()
	if err != nil || string(content) != "team\n" {
		t.Fatalf("read = %q, %v", content, err)
	}
}

func mustWorkspaceFSRun(t *testing.T, home string, args ...string) {
	t.Helper()
	result := Run(append([]string{"--home", home}, args...))
	if result.Status != 0 {
		t.Fatalf("kc %v failed: %s", args, result.Stdout)
	}
}
