package cli

import (
	"net/http/httptest"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
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

func TestPrepareRemoteWorkspaceFSUsesGatewayAndKeepsFixedPin(t *testing.T) {
	home := testkit.TempDir(t)
	project := testkit.TempDir(t)
	repository := "kr://acme/remote-docs"
	catalogID := "kr://acme/catalog"
	mustWorkspaceFSRun(t, home, "init", "--catalog", catalogID)
	mustWorkspaceFSRun(t, home, "repo-add", "--repo", repository)
	mustWorkspaceFSRun(t, home, "define-workspace", "--workspace", "agent", "--revision", "1",
		"--source", repository+"=refs/heads/main@knowledge@shared")
	mustRawTreeWrite(t, home, repository, "remote-v1", "shared/README.md", "v1\n")
	for _, args := range [][]string{
		{"admin", "grant", "add", "--principal", "agent:test", "--action", "workspace.resolve", "--catalog", catalogID, "--workspace", "agent"},
		{"admin", "grant", "add", "--principal", "agent:test", "--action", "file.read", "--repo", repository},
	} {
		if result := Run(append([]string{"--home", home}, args...)); result.Status != 0 {
			t.Fatalf("grant failed: %s", result.Stdout)
		}
	}
	server := httptest.NewServer(HTTPHandler(home))
	defer server.Close()
	plan, manifest, closeClient, err := prepareWorkspaceFS(workspaceFSConfig{
		server: server.URL, catalogID: catalogID, workspace: "agent", root: project, principal: "agent:test",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeClient()
	if len(plan.Mounts) != 1 || manifest.PinID == "" || plan.PinID != manifest.PinID {
		t.Fatalf("remote plan=%#v manifest=%#v", plan, manifest)
	}
	children, err := plan.Mounts[0].Directory.List("")
	if err != nil || len(children) != 1 || children[0].Name != "README.md" {
		t.Fatalf("remote directory=%#v err=%v", children, err)
	}
	mustRawTreeWrite(t, home, repository, "remote-v2", "shared/README.md", "v2\n")
	content, err := plan.Mounts[0].Directory.Read("README.md")
	if err != nil || string(content) != "v1\n" {
		t.Fatalf("remote pinned read=%q err=%v", content, err)
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
	mustRawTreeWrite(t, home, one, "team-file", "team/README.md", "team\n")
	mustRawTreeWrite(t, home, two, "policy-file", "rules.md", "policy\n")
	mustRawTreeWrite(t, home, one, "runbook-file", "runbooks/incident.md", "incident\n")

	plan, manifest, closeHome, err := prepareWorkspaceFS(workspaceFSConfig{
		home: home, workspace: "agent", root: project,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeHome()
	if len(plan.Mounts) != 3 || len(manifest.Mounts) != 3 {
		t.Fatalf("plan=%#v manifest=%#v", plan.Mounts, manifest.Mounts)
	}
	if plan.PinID == "" || plan.PinID != manifest.PinID {
		t.Fatalf("pin mismatch: %#v %#v", plan.PinID, manifest.PinID)
	}
	if plan.Mounts[0].Directory == nil || len(plan.Mounts[0].Files) != 0 {
		t.Fatalf("first mount must be lazy: %#v", plan.Mounts[0])
	}
	children, err := plan.Mounts[0].Directory.List("")
	if err != nil || len(children) != 1 || children[0].Name != "README.md" || children[0].Directory {
		t.Fatalf("root directory = %#v, %v", children, err)
	}
	content, err := plan.Mounts[0].Directory.Read("README.md")
	if err != nil || string(content) != "team\n" {
		t.Fatalf("read = %q, %v", content, err)
	}
}

func mustWorkspaceFSRun(t *testing.T, home string, args ...string) {
	t.Helper()
	result := Run(append([]string{"--home", home}, groupedWorkspaceFSTestArgs(args)...))
	if result.Status != 0 {
		t.Fatalf("kc %v failed: %s", args, result.Stdout)
	}
}

func groupedWorkspaceFSTestArgs(args []string) []string {
	paths := map[string][]string{
		"init": {"local", "init"}, "repo-add": {"local", "repository", "attach"},
		"define-workspace": {"catalog", "workspace", "define"},
	}
	if len(args) > 0 && len(paths[args[0]]) > 0 {
		return append(append([]string{}, paths[args[0]]...), args[1:]...)
	}
	return args
}

func mustRawTreeWrite(t *testing.T, home, repository, commandID, path, content string) {
	t.Helper()
	opened, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	store, ok := opened.Store.Get(kernel.RepositoryID(repository))
	if !ok {
		t.Fatalf("repository %s not open", repository)
	}
	base, err := store.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	_, err = opened.TreeWriter.Commit(commandID, snapshot.TreeChangeSet{
		TargetRepository: kernel.RepositoryID(repository), TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base,
		Changes: []snapshot.TreeChange{{Path: path, Content: []byte(content)}},
	})
	if err != nil {
		t.Fatal(err)
	}
}
