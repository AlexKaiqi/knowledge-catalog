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
	t.Chdir(root)
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
}

func quoted(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
