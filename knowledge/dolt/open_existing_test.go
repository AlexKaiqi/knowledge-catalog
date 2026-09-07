package dolt_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/kernel"
	knowledgedolt "kc/knowledge/dolt"
	"kc/snapshot"
	snapshotdolt "kc/snapshot/dolt"
)

func TestOpenExistingKnowledgeDoltDoesNotInstallOrMigrate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		tables     string
		compatible bool
		absent     bool
	}{
		{"absent native schema", `{"rows":[{"Tables_in_repo":"kc_files"}]}`, false, true},
		{"partial native schema", `{"rows":[{"Tables_in_repo":"kc_units"}]}`, false, false},
		{"incompatible native schema", `{"rows":[{"Tables_in_repo":"kc_units"},{"Tables_in_repo":"kc_objects"}]}`, false, false},
		{"compatible native schema without migration stamp", `{"rows":[{"Tables_in_repo":"kc_units"},{"Tables_in_repo":"kc_objects"}]}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, ".dolt"), 0o755); err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(t.TempDir(), "dolt")
			nativeResult := "printf '%s\\n' 'native table or column missing' >&2; exit 48"
			if tc.compatible {
				nativeResult = "printf '%s\\n' '{\"rows\":[]}'"
			}
			script := "#!/bin/sh\ncase \"$*\" in\n" +
				"  \"sql -r json -q SELECT DOLT_HASHOF('main') AS hash\") printf '%s\\n' '{\"rows\":[{\"hash\":\"published\"}]}' ;;\n" +
				"  \"sql -r json -q SELECT hash FROM dolt_branches WHERE name='kc-archived'\") printf '%s\\n' '{\"rows\":[]}' ;;\n" +
				"  \"sql -r json -q SHOW TABLES AS OF 'published'\") printf '%s\\n' '" + tc.tables + "' ;;\n" +
				"  \"sql -r json -q SELECT \"*\" AS OF 'published' LIMIT 0\") " + nativeResult + " ;;\n" +
				"  *) printf '%s\\n' \"unexpected or mutating command: $*\" >&2; exit 49 ;;\nesac\n"
			if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("KC_DOLT_BIN", bin)
			_, err := knowledgedolt.OpenExisting(root, "kr://acme/source")
			if tc.compatible && err != nil {
				t.Fatalf("compatible existing source: %v", err)
			}
			if !tc.compatible && kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
				t.Fatalf("incompatible source error = %v, want CAPABILITY_UNSATISFIED", err)
			}
			if got := errors.Is(err, knowledgedolt.ErrNotNativeKnowledge); got != tc.absent {
				t.Fatalf("safe Snapshot fallback = %v, want %v (error %v)", got, tc.absent, err)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".kc") {
					t.Fatalf("open wrote source metadata %s", entry.Name())
				}
			}
		})
	}
}

func TestOpenExistingNativeDoltPreservesPublishedAuthority(t *testing.T) {
	requireRuntime(t)
	root := t.TempDir()
	id := kernel.RepositoryID("kr://acme/existing-native")
	// Creating the fixture is explicit. Attempting to open its Snapshot as
	// native Knowledge must not install the absent tables.
	base, err := snapshotdolt.OpenDolt(root, id)
	if err != nil {
		t.Fatal(err)
	}
	head, err := base.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := knowledgedolt.OpenExisting(root, id); !errors.Is(err, knowledgedolt.ErrNotNativeKnowledge) {
		t.Fatalf("ordinary Snapshot must not be converted to native Knowledge: %v", err)
	}
	if after, err := base.Head(snapshot.DefaultRef); err != nil || after != head {
		t.Fatalf("failed native open changed HEAD: %s -> %s, %v", head, after, err)
	}
	if rows, err := base.NativeQuery("SHOW TABLES LIKE 'kc_units'"); err != nil || len(rows) != 0 {
		t.Fatalf("failed native open installed Knowledge tables: %#v, %v", rows, err)
	}
	// The explicit creation path installs native schema; restoration does not
	// replay that process, even when its migration stamp has been removed.
	created, err := knowledgedolt.Open(root, id)
	if err != nil {
		t.Fatal(err)
	}
	head, err = created.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	stamp := filepath.Join(root, ".kc-knowledge-format-v2")
	if err := os.Remove(stamp); err != nil {
		t.Fatal(err)
	}
	opened, err := knowledgedolt.OpenExisting(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if after, err := opened.Head(snapshot.DefaultRef); err != nil || after != head {
		t.Fatalf("restoration changed HEAD: %s -> %s, %v", head, after, err)
	}
	if _, err := os.Stat(stamp); !os.IsNotExist(err) {
		t.Fatalf("restoration recreated migration stamp: %v", err)
	}
}
