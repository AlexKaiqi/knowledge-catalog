package dolt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenExistingDoltLeavesMissingAuthorityAbsent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	if _, err := OpenExisting(root, "kr://acme/source"); err == nil {
		t.Fatal("missing Dolt authority was accepted")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("opening missing authority created its directory: %v", err)
	}
}

func TestOpenExistingDoltOnlyReadsAuthorityAndNeverStampsIt(t *testing.T) {
	for _, tc := range []struct {
		name       string
		stamp      string
		missingRef bool
		wantError  bool
	}{
		{name: "existing unstamped snapshot"},
		{name: "matching identity", stamp: "kr://acme/source\n"},
		{name: "conflicting identity", stamp: "kr://acme/other\n", wantError: true},
		{name: "missing published ref", missingRef: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, ".dolt"), 0o755); err != nil {
				t.Fatal(err)
			}
			stampPath := filepath.Join(root, doltStamp)
			if tc.stamp != "" {
				if err := os.WriteFile(stampPath, []byte(tc.stamp), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			bin := filepath.Join(t.TempDir(), "dolt")
			script := "#!/bin/sh\ncase \"$*\" in\n" +
				"  \"sql -r json -q SELECT DOLT_HASHOF('main') AS hash\") printf '%s\\n' '{\"rows\":[{\"hash\":\"published\"}]}' ;;\n" +
				"  \"sql -r json -q SELECT hash FROM dolt_branches WHERE name='kc-archived'\") printf '%s\\n' '{\"rows\":[]}' ;;\n" +
				"  *) printf '%s\\n' \"unexpected or mutating command: $*\" >&2; exit 49 ;;\nesac\n"
			if tc.missingRef {
				script = "#!/bin/sh\nprintf '%s\\n' 'published ref missing' >&2\nexit 48\n"
			}
			if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("KC_DOLT_BIN", bin)
			_, err := OpenExisting(root, "kr://acme/source")
			if (err != nil) != tc.wantError {
				t.Fatalf("OpenExisting error = %v, want error %v", err, tc.wantError)
			}
			got, err := os.ReadFile(stampPath)
			if tc.stamp == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("opening existing source wrote an identity stamp: %q, %v", got, err)
				}
			} else if err != nil || string(got) != tc.stamp {
				t.Fatalf("source identity stamp changed: %q, %v", got, err)
			}
		})
	}
}
