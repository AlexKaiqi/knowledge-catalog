package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllowedRepoReadFailsClosedWhenPolicyCannotBeRead(t *testing.T) {
	home := t.TempDir()
	// ReadAllow expects a JSON file here. A directory produces a stable read
	// error even when the test process has elevated filesystem permissions.
	if err := os.Mkdir(filepath.Join(home, "allow.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{"as": "alice"}
	if allowedRepoRead(home, flags, "kr://acme/private/core", "") {
		t.Fatal("an unreadable authorization policy must fail closed")
	}
}
