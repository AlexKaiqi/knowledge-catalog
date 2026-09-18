package dolt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kc/kernel"
)

// fakeDoltEngine is a stand-in dolt binary that records one line per process
// launch. It answers both call shapes so the same fixture measures the
// per-query process transport and a reused session transport:
//
//	dolt sql -r json -q <query>     one query, one process
//	dolt sql -r json --continue     statements on stdin, one line per result
func fakeDoltEngine(t *testing.T) (bin, launches string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "dolt")
	launches = filepath.Join(dir, "launches")
	script := `#!/bin/sh
printf '%s\n' "$*" >> ` + launches + `
answer() {
  case "$1" in
    *kc_session_ack*) printf '{"rows": [{"kc_session_ack":"%s"}]}\n' "$1" ;;
    *DOLT_HASHOF*) printf '%s\n' '{"rows": [{"hash":"published"}]}' ;;
    *dolt_branches*) printf '%s\n' '{"rows": []}' ;;
    *SHOW\ TABLES*) printf '%s\n' '{"rows": [{"Tables_in_repo":"kc_files"}]}' ;;
    *) printf '%s\n' "unexpected read: $1" >&2 ;;
  esac
}
case "$*" in
  "sql -r json --continue")
    while IFS= read -r statement; do answer "${statement%;}"; done ;;
  "sql -r json -q "*) answer "$5" ;;
  *) printf '%s\n' "unexpected or mutating command: $*" >&2; exit 49 ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KC_DOLT_BIN", bin)
	return bin, launches
}

func engineLaunches(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	lines := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func existingDoltAuthority(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".dolt"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// Every read on one Repository resolves through the same engine, so repeated
// reads must not each pay a dolt process start. A per-query process turns a
// bounded read into engine startup cost that grows with call count and is
// unrelated to the amount of knowledge stored.
func TestRepeatedReadsReuseOneEngineProcess(t *testing.T) {
	root := existingDoltAuthority(t)
	_, launches := fakeDoltEngine(t)
	repo, err := OpenExisting(root, "kr://acme/source")
	if err != nil {
		t.Fatal(err)
	}
	opened := len(engineLaunches(t, launches))
	for range 20 {
		if _, err := repo.Head("refs/heads/main"); err != nil {
			t.Fatal(err)
		}
	}
	reads := len(engineLaunches(t, launches)) - opened
	if reads != 0 {
		t.Fatalf("20 repeated reads started %d additional dolt processes; a reused engine session starts none", reads)
	}
}

// Provider SQL spans lines, and a diagnostic echoes the failing statement, so
// a failure report is multi-line too. Reading replies line by line would let
// one statement's diagnostic answer the next statement.
func TestSessionKeepsMultiLineStatementsAndDiagnosticsAligned(t *testing.T) {
	root := existingDoltAuthority(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "dolt")
	// A statement selecting from an absent table fails with a diagnostic that
	// repeats the statement, one stderr line per source line.
	script := `#!/bin/sh
answer() {
  case "$1" in
    *kc_session_ack*) printf '{"rows": [{"kc_session_ack":"%s"}]}\n' "$1" ;;
    *DOLT_HASHOF*) printf '%s\n' '{"rows": [{"hash":"published"}]}' ;;
    *dolt_branches*) printf '%s\n' '{"rows": []}' ;;
    *kc_units*) printf '%s\n' '{"rows": [{"unit":"one"}]}' ;;
    *) printf 'error on line 1 for query %s: table not found\n' "$1" >&2 ;;
  esac
}
case "$*" in
  "sql -r json --continue")
    statement=""
    while IFS= read -r line; do
      if [ -n "$statement" ]; then statement="$statement
$line"; else statement="$line"; fi
      case "$line" in
        *\;) answer "${statement%;}"; statement="" ;;
      esac
    done ;;
  "sql -r json -q "*) answer "$5" ;;
  *) printf '%s\n' "unexpected or mutating command: $*" >&2; exit 49 ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KC_DOLT_BIN", bin)
	repo, err := OpenExisting(root, "kr://acme/source")
	if err != nil {
		t.Fatal(err)
	}
	multiLine := "SELECT missing\n  FROM absent_table\n  WHERE object_key='k'"
	if _, err := repo.NativeQuery(multiLine); err == nil {
		t.Fatal("statement over an absent table must fail its own call")
	}
	rows, err := repo.NativeQuery("SELECT unit FROM kc_units\n  WHERE object_key='k'")
	if err != nil {
		t.Fatalf("read after a multi-line diagnostic: %v", err)
	}
	if len(rows) != 1 || rows[0]["unit"] != "one" {
		t.Fatalf("a prior diagnostic answered the next statement: %#v", rows)
	}
}

// The session must not become a second source of truth for results or errors.
// A failing statement stays attributable to its own call and the following
// read still answers from the same engine.
func TestSessionKeepsPerStatementResultsAndErrorsAttributable(t *testing.T) {
	root := existingDoltAuthority(t)
	_, launches := fakeDoltEngine(t)
	repo, err := OpenExisting(root, "kr://acme/source")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.NativeQuery("SELECT this is not answerable"); err == nil {
		t.Fatal("unanswerable statement must fail its own call")
	}
	rows, err := repo.NativeQuery("SELECT DOLT_HASHOF('main') AS hash")
	if err != nil || len(rows) != 1 || rows[0]["hash"] != "published" {
		t.Fatalf("read after a failed statement must stay aligned: %#v %v", rows, err)
	}
	if commit, err := repo.Head("refs/heads/main"); err != nil || commit != kernel.CommitID("published") {
		t.Fatalf("head after a failed statement: %s %v", commit, err)
	}
	if got := len(engineLaunches(t, launches)); got > 2 {
		t.Fatalf("a failed statement must not restart the engine per call: %d launches", got)
	}
}

// stdout and stderr normally start as different file descriptors, so two
// reader goroutines cannot use channel arrival order as statement order. The
// transport must merge them before reading so one diagnostic cannot be
// consumed by the following query.
func TestSessionDoesNotLetDiagnosticPoisonNextQuery(t *testing.T) {
	root := existingDoltAuthority(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "dolt")
	script := `#!/bin/sh
answer() {
  case "$1" in
    *kc_session_ack*) printf '{"rows":[{"kc_session_ack":"%s"}]}\n' "$1" ;;
    *missing_table*) printf '%s\n' 'table not found: missing_table' >&2 ;;
    *DOLT_HASHOF*) printf '%s\n' '{"rows":[{"hash":"published"}]}' ;;
    *dolt_branches*) printf '%s\n' '{"rows":[]}' ;;
    *) printf '%s\n' "unexpected query: $1" >&2 ;;
  esac
}
case "$*" in
  "sql -r json --continue")
    while IFS= read -r statement; do answer "${statement%;}"; done ;;
  *) exit 49 ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KC_DOLT_BIN", bin)
	repo, err := OpenExisting(root, "kr://acme/source")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.NativeQuery("SELECT value FROM missing_table"); err == nil {
		t.Fatal("failed statement must report its diagnostic")
	}
	rows, err := repo.NativeQuery("SELECT DOLT_HASHOF('main') AS hash")
	if err != nil {
		t.Fatalf("prior diagnostic poisoned the next query: %v", err)
	}
	if len(rows) != 1 || rows[0]["hash"] != "published" {
		t.Fatalf("read after delayed diagnostic: %#v", rows)
	}
}

func TestCloseInterruptsUnacknowledgedSessionAndReapsProcess(t *testing.T) {
	root := existingDoltAuthority(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "dolt")
	started := filepath.Join(dir, "started")
	script := `#!/bin/sh
answer() {
  case "$1" in
    *kc_session_ack*) printf '{"rows":[{"kc_session_ack":"%s"}]}\n' "$1" ;;
    *DOLT_HASHOF*) printf '%s\n' '{"rows":[{"hash":"published"}]}' ;;
    *dolt_branches*) printf '%s\n' '{"rows":[]}' ;;
    *WAIT_FOREVER*) touch ` + started + `; while :; do sleep 1; done ;;
  esac
}
while IFS= read -r statement; do answer "${statement%;}"; done
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KC_DOLT_BIN", bin)
	repo, err := OpenExisting(root, "kr://acme/source")
	if err != nil {
		t.Fatal(err)
	}
	queryDone := make(chan error, 1)
	go func() {
		_, err := repo.NativeQuery("SELECT WAIT_FOREVER")
		queryDone <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session did not start the unacknowledged statement")
		}
		time.Sleep(10 * time.Millisecond)
	}
	closed := make(chan struct{})
	go func() {
		closeEngineAt(root)
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("session close blocked behind an unacknowledged query")
	}
	select {
	case err := <-queryDone:
		if err == nil {
			t.Fatal("interrupted query unexpectedly succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("interrupted query did not observe process exit")
	}
}
