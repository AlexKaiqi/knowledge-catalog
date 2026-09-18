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
	context := `{"version":1,"principal":"agent:test","catalog":"kr://acme/catalog","dataset":"agent","pin":{"setId":"agent","revision":1,"pinId":"pin-1","repositories":{"kr://acme/docs":"c1"}},"root":` + quoted(root) + `,"readOnly":true}`
	if err := os.WriteFile(filepath.Join(dir, "context.json"), []byte(context), 0o600); err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{}
	if err := inheritTaskContext("read", flags); err != nil {
		t.Fatal(err)
	}
	if FlagString(flags, "as") != "agent:test" || FlagString(flags, "dataset") != "agent" || FlagString(flags, "pin") == "" {
		t.Fatalf("context was not inherited: %#v", flags)
	}
	conflict := map[string]FlagValue{"dataset": "other"}
	if err := inheritTaskContext("search", conflict); err == nil {
		t.Fatal("conflicting Workspace was accepted")
	}
	writer := map[string]FlagValue{}
	if err := inheritTaskContext("writer put", writer); err != nil {
		t.Fatal(err)
	}
	if FlagString(writer, "as") != "agent:test" || FlagString(writer, "dataset") != "" || FlagString(writer, "pin") != "" {
		t.Fatalf("writer must inherit identity but not consumer coordinates: %#v", writer)
	}
	maintainer := map[string]FlagValue{"repo": "kr://acme/docs"}
	if err := inheritTaskContext("read", maintainer); err != nil {
		t.Fatal(err)
	}
	if FlagString(maintainer, "as") != "agent:test" || FlagString(maintainer, "dataset") != "" || FlagString(maintainer, "pin") != "" {
		t.Fatalf("maintainer --repo read must not inherit a mounted knowledge set: %#v", maintainer)
	}
}

func TestDatasetOnlyTaskContextDoesNotRequirePin(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	chdir(t, root)
	t.Setenv("KC_HOME", home)
	dir := filepath.Join(home, "tasks", "dataset-only")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	context := `{"version":1,"principal":"agent:test","catalog":"kr://acme/catalog","dataset":"agent","root":` + quoted(root) + `,"readOnly":true}`
	if err := os.WriteFile(filepath.Join(dir, "context.json"), []byte(context), 0o600); err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{}
	if err := inheritTaskContext("read", flags); err != nil {
		t.Fatal(err)
	}
	if FlagString(flags, "as") != "agent:test" || FlagString(flags, "dataset") != "agent" || FlagString(flags, "pin") != "" {
		t.Fatalf("dataset-only context must inherit --dataset without a consumer pin: %#v", flags)
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
	raw := `{"version":1,"dataset":"","pinId":"","root":` + quoted(root) + `,"readOnly":true,"mounts":[]}`
	if err := os.WriteFile(filepath.Join(dir, "context.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"catalog list", "schema list", "read"} {
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
		"parent": `{"version":1,"principal":"agent:parent","dataset":"parent","pin":{"setId":"parent","repositories":{"kr://acme/source":"old"}},"root":` + quoted(parent) + `,"readOnly":true}`,
		"child":  `{"version":1,"dataset":"","root":` + quoted(root) + `,"readOnly":true}`,
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
	if err := inheritTaskContext("read", flags); err != nil {
		t.Fatal(err)
	}
	if len(flags) != 0 {
		t.Fatalf("unbound child inherited stale parent task: %#v", flags)
	}
}

func TestProjectUIContextOverridesUnboundTaskAndReplaysTemporaryPin(t *testing.T) {
	isolateLoginConfig(t)
	home, root := t.TempDir(), t.TempDir()
	chdir(t, root)
	t.Setenv("KC_HOME", home)
	pin := `{"setId":"","revision":1,"pinId":"pin-1","repositories":{"kr://acme/docs":"c1"},"catalog":"kr://acme/catalog","definition":{"setId":"","revision":1,"sources":[{"repository":"kr://acme/docs","selector":"refs/heads/main"}]}}`
	for group, raw := range map[string]string{
		"tasks":    `{"version":1,"dataset":"","root":` + quoted(root) + `,"readOnly":true}`,
		"projects": `{"version":1,"authMode":"token","server":"https://kc.test","dataset":"","pin":` + pin + `,"root":` + quoted(root) + `,"readOnly":true}`,
	} {
		dir := filepath.Join(home, group, "active")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "context.json"), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	flags := map[string]FlagValue{}
	if err := inheritTaskContext("read", flags); err != nil {
		t.Fatal(err)
	}
	if FlagString(flags, "server") != "https://kc.test" || FlagString(flags, "as") != "" || !sameJSON([]byte(FlagString(flags, "pin")), []byte(pin)) {
		t.Fatalf("project token context not inherited: %#v", flags)
	}
	if err := prepareKnowledgePinContext(flags); err != nil {
		t.Fatal(err)
	}
	if suppliedKnowledgeSet(flags) == nil || FlagString(flags, "dataset") != "" {
		t.Fatalf("temporary context lost definition: %#v", flags)
	}
	conflict := map[string]FlagValue{"server": "https://other.test"}
	if err := inheritTaskContext("read", conflict); err == nil {
		t.Fatal("task pin accepted conflicting server")
	}
}
