package cli

import (
	apphome "kc/home"
	serv "kc/server"
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
		{"object/team", "object/team/README.md", "README.md", true},
		{"object/team", "object/team", "", true},
		{"object/team", "object/other/a", "", false},
		{"", "README.md", "README.md", true},
	} {
		got, ok := relativeMountPath(tc.mount, tc.path)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("relativeMountPath(%q, %q) = %q, %v; want %q, %v", tc.mount, tc.path, got, ok, tc.want, tc.ok)
		}
	}
}

func TestPrepareRemoteDatasetFSUsesGatewayAndKeepsFixedPin(t *testing.T) {
	home := testkit.TempDir(t)
	project := testkit.TempDir(t)
	repository := "kr://acme/remote-docs"
	mustDatasetFSRun(t, home, "init")
	mustDatasetFSRun(t, home, "repo-add", "--repo", repository, "--dsn", internalLakeFSRepoDSN(t))
	mustDatasetFSRun(t, home, "register", "--repo", repository)
	mustRawTreeWrite(t, home, repository, "remote-v1", "shared/README.md", "v1\n")
	mustDatasetFSRun(t, home, "dataset", "define", "--dataset", "agent", "--revision", "1",
		"--source", repository+"=refs/heads/main@object@shared")
	for _, args := range [][]string{
		{"grant", "add", "--principal", "agent:test", "--action", "dataset.resolve", "--dataset", "agent"},
		{"grant", "add", "--principal", "agent:test", "--action", "file.read", "--repo", repository},
	} {
		if result := runWithTelemetryMode(append([]string{"--home", home}, args...), nil, true); result.Status != 0 {
			t.Fatalf("grant failed: %s", result.Stdout)
		}
	}
	server := httptest.NewServer(serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{}))
	defer server.Close()
	plan, manifest, closeClient, err := prepareDatasetFS(datasetFSConfig{
		server: server.URL, dataset: "agent", root: project, principal: "agent:test",
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

func TestPrepareDatasetFSMakesOneTargetPerRecipePath(t *testing.T) {
	home := testkit.TempDir(t)
	project := testkit.TempDir(t)
	one := "kr://acme/team/docs"
	two := "kr://acme/org/policy"
	mustDatasetFSRun(t, home, "init")
	mustDatasetFSRun(t, home, "repo-add", "--repo", one, "--dsn", internalLakeFSRepoDSN(t))
	mustDatasetFSRun(t, home, "register", "--repo", one)
	mustDatasetFSRun(t, home, "repo-add", "--repo", two, "--dsn", internalLakeFSRepoDSN(t))
	mustDatasetFSRun(t, home, "register", "--repo", two)
	mustRawTreeWrite(t, home, one, "team-file", "team/README.md", "team\n")
	mustRawTreeWrite(t, home, two, "policy-file", "rules.md", "policy\n")
	mustRawTreeWrite(t, home, one, "runbook-file", "runbooks/incident.md", "incident\n")
	mustDatasetFSRun(t, home, "dataset", "define", "--dataset", "agent", "--revision", "1",
		"--source", one+"=refs/heads/main@docs/team@team",
		"--source", one+"=refs/heads/main@docs/runbooks@runbooks",
		"--source", two+"=refs/heads/main@object/policy")

	plan, manifest, closeHome, err := prepareEmbeddedDatasetFS(home, datasetFSConfig{
		dataset: "agent", root: project,
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

func mustDatasetFSRun(t *testing.T, home string, args ...string) {
	t.Helper()
	result := RunEmbeddedForTest(append([]string{"--home", home}, groupedDatasetFSTestArgs(args)...), nil)
	if result.Status != 0 {
		t.Fatalf("kc %v failed: %s", args, result.Stdout)
	}
}

func groupedDatasetFSTestArgs(args []string) []string {
	paths := map[string][]string{
		"init": {"local", "init"}, "repo-add": {"local", "repository", "attach"},
		"register": {"attach"},
	}
	if len(args) > 0 && len(paths[args[0]]) > 0 {
		return append(append([]string{}, paths[args[0]]...), args[1:]...)
	}
	return args
}

func mustRawTreeWrite(t *testing.T, home, repository, commandID, path, content string) {
	t.Helper()
	opened, err := apphome.Open(home)
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
