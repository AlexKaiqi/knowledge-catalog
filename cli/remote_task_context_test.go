package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestKnowledgeCommandInheritsPrivateMountedTaskContext(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	chdir(t, root)
	t.Setenv("KC_HOME", home)
	dir := filepath.Join(home, "tasks", "task-one")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	context := `{"version":1,"principal":"agent:test","catalog":"kr://acme/catalog","workspace":"agent","pin":{"workspaceId":"agent","revision":1,"pinId":"pin-1","repositories":{"kr://acme/docs":"c1"}},"root":` + quoted(root) + `,"readOnly":true}`
	if err := os.WriteFile(filepath.Join(dir, "context.json"), []byte(context), 0o600); err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{}
	if err := inheritTaskContext("knowledge read", flags); err != nil {
		t.Fatal(err)
	}
	if FlagString(flags, "as") != "agent:test" || FlagString(flags, "workspace") != "agent" || FlagString(flags, "pin") == "" {
		t.Fatalf("context was not inherited: %#v", flags)
	}
	conflict := map[string]FlagValue{"workspace": "other"}
	if err := inheritTaskContext("knowledge search", conflict); err == nil {
		t.Fatal("conflicting Workspace was accepted")
	}
	writer := map[string]FlagValue{}
	if err := inheritTaskContext("writer put", writer); err != nil {
		t.Fatal(err)
	}
	if FlagString(writer, "as") != "agent:test" || FlagString(writer, "workspace") != "" || FlagString(writer, "pin") != "" {
		t.Fatalf("writer must inherit identity but not consumer coordinates: %#v", writer)
	}
	maintainer := map[string]FlagValue{"repo": "kr://acme/docs"}
	if err := inheritTaskContext("knowledge read", maintainer); err != nil {
		t.Fatal(err)
	}
	if FlagString(maintainer, "as") != "agent:test" || FlagString(maintainer, "workspace") != "" || FlagString(maintainer, "pin") != "" {
		t.Fatalf("maintainer --repo read must not inherit a mounted knowledge set: %#v", maintainer)
	}
}

// chdir is testing.T.Chdir from Go 1.24. The module targets 1.23, so the
// restore-on-cleanup behaviour is provided locally instead of raising the
// toolchain requirement for one test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatal(err)
		}
	})
}

func quoted(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func TestEmptyUnboundTaskContextDoesNotBlockDiscovery(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	chdir(t, root)
	t.Setenv("KC_HOME", home)
	dir := filepath.Join(home, "tasks", "unbound")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := `{"version":1,"workspace":"","pinId":"","root":` + quoted(root) + `,"readOnly":true,"mounts":[]}`
	if err := os.WriteFile(filepath.Join(dir, "context.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"catalog list", "knowledge schema list", "knowledge read"} {
		flags := map[string]FlagValue{"as": "agent:explicit"}
		if err := inheritTaskContext(path, flags); err != nil {
			t.Fatalf("%s blocked by unbound task: %v", path, err)
		}
		if len(flags) != 1 {
			t.Fatalf("unbound task injected coordinates: %#v", flags)
		}
	}
}

func TestEmptyNestedTaskStopsInheritingParentKnowledge(t *testing.T) {
	home, parent := t.TempDir(), t.TempDir()
	root := filepath.Join(parent, "child")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)
	t.Setenv("KC_HOME", home)
	for name, raw := range map[string]string{
		"parent": `{"version":1,"principal":"agent:parent","workspace":"parent","pin":{"workspaceId":"parent","repositories":{"kr://acme/source":"old"}},"root":` + quoted(parent) + `,"readOnly":true}`,
		"child":  `{"version":1,"workspace":"","root":` + quoted(root) + `,"readOnly":true}`,
	} {
		dir := filepath.Join(home, "tasks", name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "context.json"), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	flags := map[string]FlagValue{}
	if err := inheritTaskContext("knowledge read", flags); err != nil {
		t.Fatal(err)
	}
	if len(flags) != 0 {
		t.Fatalf("unbound child inherited stale parent task: %#v", flags)
	}
}
