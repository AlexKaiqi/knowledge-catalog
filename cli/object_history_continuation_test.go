package cli_test

import (
	"fmt"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestObjectHistoryEmptyPagesResumeAfterColdRestartWithOriginalScope(t *testing.T) {
	isolateClientCredentials(t)
	t.Setenv("KC_DATASET", "")
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	var historyCalls atomic.Int32
	fake := testkit.NewLakeFSFakeWithInterceptor(t, func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Query().Get("amount") != "" {
			historyCalls.Add(1)
		}
		return false
	})
	dsn := fake.DSN(fake.NewRepo())
	home := testkit.TempDir(t)
	const repo = "kr://history/continuous"
	body(t, kc(home, "local", "init"))
	seedRepo(t, home, repo, "--driver", "lakefs", "--dsn", dsn)
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", "operator"))
	body(t, kc(home, "store-set", "--index", "none"))
	url, stop := serveReferenceHome(t, home)
	admin := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	seed := asMap(t, body(t, admin("writer", "put", "urn:kc:object:note/history", "--repo", repo, "--command-id", "history-seed", "--value", `{"body":"original"}`)))
	introduction := asMap(t, seed["result"])["newCommit"].(string)
	source, err := snapshot.OpenLakeFS(kernel.RepositoryID(repo), dsn, fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	stores := snapshot.NewRegistry()
	if err := stores.Add(source); err != nil {
		t.Fatal(err)
	}
	raw, err := snapshot.NewRawWriter(stores, nil)
	if err != nil {
		t.Fatal(err)
	}
	at := kernel.CommitID(introduction)
	for i := 0; i < 780; i++ {
		receipt, err := raw.Commit(fmt.Sprintf("unrelated-history-%d", i), snapshot.TreeChangeSet{TargetRepository: kernel.RepositoryID(repo), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Changes: []snapshot.TreeChange{{Path: "maintenance-marker.txt", Content: []byte(fmt.Sprint(i))}}})
		if err != nil {
			t.Fatal(err)
		}
		at = receipt.Result.NewCommit
	}
	grant := asMap(t, body(t, admin("grant", "add", "--principal", "history-reader", "--repo", repo, "--action", "object.history.read")))
	read := func(args ...string) kcRunResult { return kcRemote(t, url, "history-reader", args...) }
	first := asMap(t, body(t, read("log", "--repo", repo, "--object", "note/history", "--limit", "1")))
	if first["complete"] != false || len(first["logs"].([]any)) != 0 || first["scanned"].(float64) <= 0 || first["scanned"].(float64) > 256 {
		t.Fatalf("unproved segment was published or scan was unbounded: %#v", first)
	}
	cursor := first["continuation"].(string)
	before := historyCalls.Load()
	expectCode(t, read("log", "--continuation", cursor, "--object", "note/other"), string(kernel.ErrUsageInvalid))
	expectCode(t, read("log", "--continuation", cursor, "--repo", repo, "--commit", introduction), string(kernel.ErrUsageInvalid))
	if historyCalls.Load() != before {
		t.Fatal("mismatched continuation scanned history")
	}
	matching := asMap(t, body(t, read("log", "--continuation", cursor, "--repo", repo, "--commit", string(at), "--ref", snapshot.DefaultRef, "--object", "note/history", "--limit", "1")))
	if matching["complete"] != false || matching["continuation"] == cursor {
		t.Fatalf("matching explicit coordinates were not restored: %#v", matching)
	}
	body(t, admin("grant", "remove", "--id", grant["id"].(string)))
	expectCode(t, read("log", "--continuation", cursor), string(kernel.ErrForbidden))
	body(t, admin("grant", "add", "--principal", "history-reader", "--repo", repo, "--action", "object.history.read"))
	body(t, admin("writer", "put", "urn:kc:object:note/history", "--repo", repo, "--command-id", "history-after-first-page", "--value", `{"body":"newer"}`))
	stop()
	url, _ = serveReferenceHome(t, home)
	t.Setenv("KC_DATASET", "unrelated-client-default")
	empty := 1
	complete := false
	var revisions []string
	for pageNumber := 0; pageNumber < 64; pageNumber++ {
		page := asMap(t, body(t, read("log", "--continuation", cursor, "--limit", "1")))
		for _, entry := range page["logs"].([]any) {
			log := asMap(t, entry)
			if log["commit"] != string(at) {
				t.Fatalf("continuation followed a new source head: %#v", log)
			}
			for _, entry := range log["revisions"].([]any) {
				revisions = append(revisions, asMap(t, entry)["commit"].(string))
			}
		}
		if page["complete"] == true {
			if _, ok := page["continuation"]; ok {
				t.Fatal("completed history retained a cursor")
			}
			complete = true
			break
		}
		if len(page["logs"].([]any)) == 0 {
			empty++
		}
		next := page["continuation"].(string)
		if next == "" || next == cursor {
			t.Fatal("empty continuation did not advance")
		}
		cursor = next
	}
	if !complete || empty < 3 || len(revisions) != 1 || revisions[0] != introduction {
		t.Fatalf("history lost true introduction or empty progress: complete=%v empty=%d revisions=%v", complete, empty, revisions)
	}
}

func TestDatasetHistoryContinuationFreezesPublicationAndRechecksDatasetGrant(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	admin := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	for i, value := range []string{"first", "second"} {
		body(t, admin("writer", "put", "urn:kc:object:note/history", "--repo", repo, "--command-id", fmt.Sprintf("dataset-history-%d", i), "--value", fmt.Sprintf(`{"body":%q}`, value)))
	}
	body(t, admin("dataset", "define", "--dataset", "history-release", "--revision", "1", "--source", repo))
	body(t, admin("grant", "add", "--principal", "history-consumer", "--repo", repo, "--action", "object.history.read"))
	grant := asMap(t, body(t, admin("grant", "add", "--principal", "history-consumer", "--dataset", "history-release", "--action", "file.read")))
	read := func(args ...string) kcRunResult { return kcRemote(t, url, "history-consumer", args...) }
	first := asMap(t, body(t, read("log", "--dataset", "history-release", "--object", "note/history", "--limit", "1")))
	logs := first["logs"].([]any)
	if len(logs) != 1 || first["complete"] != false {
		t.Fatalf("missing first history page: %#v", first)
	}
	fixed := asMap(t, logs[0])["commit"]
	cursor := first["continuation"].(string)
	body(t, admin("writer", "put", "urn:kc:object:note/history", "--repo", repo, "--command-id", "dataset-history-later", "--value", `{"body":"third"}`))
	body(t, admin("dataset", "define", "--dataset", "history-release", "--revision", "2", "--source", repo))
	next := asMap(t, body(t, read("log", "--continuation", cursor, "--revision", "1", "--limit", "1")))
	if entries := next["logs"].([]any); len(entries) != 1 || asMap(t, entries[0])["commit"] != fixed {
		t.Fatalf("history continuation followed a new Dataset publication: %#v", next)
	}
	expectCode(t, read("log", "--continuation", cursor, "--dataset", "history-release", "--revision", "2"), string(kernel.ErrUsageInvalid))
	body(t, admin("grant", "remove", "--id", grant["id"].(string)))
	expectCode(t, read("log", "--continuation", cursor), string(kernel.ErrForbidden))
	body(t, read("log", "--repo", repo, "--object", "note/history", "--limit", "1"))
	body(t, admin("grant", "add", "--principal", "history-consumer", "--dataset", "history-release", "--action", "file.read"))
	body(t, admin("dataset", "retire", "--dataset", "history-release"))
	expectCode(t, read("log", "--continuation", cursor), string(kernel.ErrDatasetInvalid))
}
