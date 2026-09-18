package dolt

import (
	"encoding/json"
	"kc/snapshot"
	"os"
	"path/filepath"
	"testing"
)

func TestManagedDoltNeverAdoptsExistingDirectory(t *testing.T) {
	for _, populated := range []bool{false, true} {
		root := t.TempDir()
		if populated {
			if err := os.Mkdir(filepath.Join(root, ".dolt"), 0700); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := CreateManaged(root, "kr://managed/repo", "allocation"); err == nil {
			t.Fatal("managed create adopted an external directory")
		}
		if _, err := os.Stat(filepath.Join(root, doltStamp)); !os.IsNotExist(err) {
			t.Fatal("managed create stamped external authority")
		}
	}
}

func TestManagedDoltResumesInterruptedOwnedBootstrap(t *testing.T) {
	root := filepath.Join(t.TempDir(), "allocated")
	bin := filepath.Join(t.TempDir(), "dolt")
	script := `#!/bin/sh
answer() {
  case "$1" in
    *kc_session_ack*) printf '{"rows":[{"kc_session_ack":"%s"}]}\n' "$1" ;;
    "SELECT DOLT_HASHOF('main') AS hash")
      printf '%s\n' '{"rows":[{"hash":"initial"}]}' ;;
    "SELECT hash FROM dolt_branches WHERE name='kc-archived'")
      printf '%s\n' '{"rows":[]}' ;;
    "SHOW TABLES LIKE 'kc_files'")
      if test -e table-installed; then printf '%s\n' '{"rows":[{"name":"kc_files"}]}'
      else printf '%s\n' '{"rows":[]}'; fi ;;
    *) printf 'unexpected read: %s\n' "$1" >&2 ;;
  esac
}
case "$*" in
  init\ *)
    test ! -e initialized || exit 51
    mkdir .dolt
    touch initialized
    ;;
  "sql -q CREATE TABLE kc_files (path VARCHAR(1024) PRIMARY KEY, content LONGBLOB NOT NULL)")
    exit 52
    ;;
  "sql -r json --continue")
    while IFS= read -r statement; do answer "${statement%;}"; done ;;
  "sql -r json -q "*) answer "$5" ;;
  "sql -q CREATE TABLE IF NOT EXISTS kc_files (path VARCHAR(1024) PRIMARY KEY, content LONGBLOB NOT NULL)")
    touch table-installed
    ;;
  "checkout main"|"add ."|"commit --allow-empty -m install native knowledge schema") ;;
  *) printf 'unexpected command: %s\n' "$*" >&2; exit 53 ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KC_DOLT_BIN", bin)
	if _, err := CreateManaged(root, "kr://managed/repo", "allocation"); err == nil {
		t.Fatal("initial table-installation failure was hidden")
	}
	if err := VerifyManaged(root, "kr://managed/repo", "allocation"); err != nil {
		t.Fatal("interruption lost durable ownership", err)
	}
	repo, err := CreateManaged(root, "kr://managed/repo", "allocation")
	if err != nil {
		t.Fatal("owned bootstrap did not resume", err)
	}
	if _, err := os.Stat(filepath.Join(root, "table-installed")); err != nil {
		t.Fatal("owned retry did not install missing raw table")
	}
	if head, err := repo.Head(snapshot.DefaultRef); err != nil || head != "initial" {
		t.Fatalf("retry head: %s %v", head, err)
	}
	if _, err := CreateManaged(root, "kr://managed/repo", "other"); err == nil {
		t.Fatal("another allocation adopted partial bootstrap")
	}
}

func TestManagedDoltOpenedHandleRejectsLostOwnershipForNativeQuery(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".dolt"), 0700); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(managedOwnership{Repository: "kr://managed/repo", Allocation: "allocation"})
	if err := os.WriteFile(filepath.Join(root, managedMarker), body, 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "dolt")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' '{\"rows\":[{\"hash\":\"initial\"}]}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KC_DOLT_BIN", bin)
	repo, err := OpenManaged(root, "kr://managed/repo", "allocation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.NativeQuery("SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, managedMarker)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.NativeQuery("SELECT 1"); err == nil {
		t.Fatal("native query used managed directory after ownership loss")
	}
}
