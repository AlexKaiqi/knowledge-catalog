package dolt_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	knowledgedolt "kc/knowledge/dolt"
	"kc/retrieval/cache"
)

func nativeReadVersionFixture(t *testing.T) (*knowledgedolt.Repository, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".dolt"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "dolt")
	queryLog := filepath.Join(t.TempDir(), "queries.log")
	// The log records one line per statement, not per process, so query-count
	// assertions stay about queries when reads share one engine session.
	script := `#!/bin/sh
answer() {
  case "$1" in
    *kc_session_ack*) printf '{"rows":[{"kc_session_ack":"%s"}]}\n' "$1"; return 0 ;;
  esac
  printf '%s\n' "$1" >> "$KC_NATIVE_READ_QUERY_LOG"
  case "$1" in
    "SELECT DOLT_HASHOF('main') AS hash"|"SELECT DOLT_HASHOF('published') AS hash")
      printf '%s\n' '{"rows":[{"hash":"published"}]}' ;;
    "SELECT DOLT_HASHOF('missing-commit') AS hash")
      printf '%s\n' 'branch not found: missing-commit' >&2; return 1 ;;
    "SELECT hash FROM dolt_branches WHERE name='kc-archived'")
      printf '%s\n' '{"rows":[]}' ;;
    "SHOW TABLES AS OF 'published'")
      printf '%s\n' '{"rows":[{"Tables_in_repo":"kc_units"},{"Tables_in_repo":"kc_objects"}]}' ;;
    "SELECT"*" AS OF 'published' LIMIT 0")
      printf '%s\n' '{"rows":[]}' ;;
    "SELECT"*" FROM kc_objects AS OF 'published' WHERE object_key='__A_KEY__' LIMIT 1")
      printf '%s\n' '{"rows":[{"kind":"Entity","status":"RESOLVED","object_id64":"YQ=="}]}' ;;
    "SELECT"*" AS OF 'published' WHERE object_key IN ('__A_KEY__') ORDER BY object_key, unit_key")
      printf '%s\n' '{"rows":[{"object_key":"__A_KEY__","kind":"Entity","object_id64":"YQ==","value_json64":"eyJ2IjoxfQ=="}]}' ;;
    "SELECT"*" AS OF 'published' WHERE object_key IN ("*)
      printf '%s\n' '{"rows":[]}' ;;
    "SELECT"*" AS OF 'missing-commit' "*)
      printf '%s\n' 'branch not found: missing-commit' >&2; return 1 ;;
    *) printf '%s\n' "unexpected query: $1" >&2; return 1 ;;
  esac
}
case "$*" in
  "sql -r json --continue")
    statement=""
    while IFS= read -r line; do
      if [ -n "$statement" ]; then statement="$statement
$line"; else statement="$line"; fi
      case "$line" in
        *\;) answer "${statement%;}" || true; statement="" ;;
      esac
    done ;;
  "sql -r json -q "*) answer "$5" || exit 1 ;;
  *) printf '%s\n' "unexpected command: $*" >&2; exit 49 ;;
esac
`
	script = strings.ReplaceAll(script, "__A_KEY__", string(kernel.CanonicalDigest(map[string]any{"objectId": knowledge.ObjectID("a")})))
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KC_DOLT_BIN", bin)
	t.Setenv("KC_NATIVE_READ_QUERY_LOG", queryLog)
	r, err := knowledgedolt.OpenExisting(root, "kr://native/version-contract")
	if err != nil {
		t.Fatal(err)
	}
	return r, queryLog
}

func TestNativeBatchReadRejectsUnresolvedCommitLikeSingleRead(t *testing.T) {
	r, _ := nativeReadVersionFixture(t)
	const absent kernel.CommitID = "missing-commit"
	if _, err := r.Read("a", absent); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("single read: got %v, want VERSION_UNRESOLVED", err)
	}
	if _, err := r.ReadMany([]knowledge.ObjectID{"a", "b"}, absent); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("batch read: got %v, want VERSION_UNRESOLVED", err)
	}
	c, err := cache.New(cache.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReadMany(r, absent, []knowledge.ObjectID{"a"}); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("cached batch miss: got %v, want VERSION_UNRESOLVED", err)
	}
	if c.Stats().Entries != 0 {
		t.Fatal("invalid commit was retained by cache")
	}
}

func TestNativeBatchVersionCheckPreservesBoundedAndEmptyReads(t *testing.T) {
	r, queryLog := nativeReadVersionFixture(t)
	if err := os.WriteFile(queryLog, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := r.ReadMany([]knowledge.ObjectID{"missing-a", "removed-b"}, "published")
	if err != nil || len(values) != 0 {
		t.Fatalf("known version with absent units: values=%v err=%v", values, err)
	}
	raw, err := os.ReadFile(queryLog)
	if err != nil {
		t.Fatal(err)
	}
	log := string(raw)
	if strings.Count(log, "DOLT_HASHOF('published')") != 1 || strings.Count(log, "WHERE object_key IN (") != 1 {
		t.Fatalf("batch must verify one commit and issue one bounded unit query:\n%s", log)
	}
	if err := os.WriteFile(queryLog, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if values, err := r.ReadMany(nil, "missing-commit"); err != nil || len(values) != 0 {
		t.Fatalf("empty batch: values=%v err=%v", values, err)
	}
	raw, err = os.ReadFile(queryLog)
	if err != nil || len(raw) != 0 {
		t.Fatalf("empty batch must issue no query: %s, %v", raw, err)
	}
}

func TestNativeSingleReadChecksCommitOnlyOnce(t *testing.T) {
	r, queryLog := nativeReadVersionFixture(t)
	if err := os.WriteFile(queryLog, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := r.Read("a", "published")
	if err != nil {
		t.Fatal(err)
	}
	if value.KnowledgeRef.Object != "a" || value.Commit != "published" || value.Value.(map[string]any)["v"] == nil {
		t.Fatalf("valid single read: %+v", value)
	}
	raw, err := os.ReadFile(queryLog)
	if err != nil {
		t.Fatal(err)
	}
	log := string(raw)
	if strings.Count(log, "DOLT_HASHOF('published')") != 1 || strings.Count(log, "WHERE object_key IN (") != 1 {
		t.Fatalf("single read must reuse its manifest's version check:\n%s", log)
	}
}
