package cli_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kc/client"
	"kc/kernel"
	objectmodel "kc/object"
	objectclient "kc/object/client"
)

func TestNoChangeObjectCLIAndHTTPRecoveryKeepChangedFalse(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	ref := ordersReference(t, repo, "")
	seed := asMap(t, body(t, run("writer", "put", ref, "--repo", repo, "--command-id", "no-change-seed", "--value", `{"body":"same"}`)))
	if asMap(t, seed["result"])["changed"] != true {
		t.Fatalf("changed write omitted verdict: %#v", seed)
	}
	first := asMap(t, body(t, run("writer", "put", ref, "--repo", repo, "--command-id", "no-change-cli", "--value", `{"body":"same"}`)))
	result := asMap(t, first["result"])
	if result["changed"] != false || result["newCommit"] != asMap(t, seed["result"])["newCommit"] || result["oldCommit"] != result["newCommit"] {
		t.Fatalf("CLI no-change receipt: %#v", first)
	}
	lookup := asMap(t, body(t, run("writer", "receipt", "--command-id", "no-change-cli")))
	if asMap(t, asMap(t, lookup["receipt"])["result"])["changed"] != false {
		t.Fatalf("receipt query lost no-change verdict: %#v", lookup)
	}
	loss := &loseAcceptedObjectResponse{}
	remote, err := client.New(client.Config{BaseURL: url, HTTPClient: &http.Client{Transport: loss}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "operator"}}); err != nil {
		t.Fatal(err)
	}
	recovered, err := objectclient.MaintainWithReceiptRecovery(context.Background(), remote.ObjectService(), false, objectclient.MaintenanceRequest{Reference: ref, Repository: repo, CommandID: "no-change-response-lost", Value: map[string]any{"body": "same"}}, objectclient.RequestOptions{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := recovered.(objectmodel.CommitReceipt)
	if !ok || receipt.Result.Changed != objectmodel.ChangeUnchanged || string(receipt.Result.NewCommit) != result["newCommit"] || receipt.ReceiptRef == first["receiptRef"] {
		t.Fatalf("HTTP recovery lost false or command identity: %#v", recovered)
	}
	loss.mu.Lock()
	posts := loss.posts
	loss.mu.Unlock()
	if posts != 1 {
		t.Fatalf("no-change response loss repeated mutation: %d", posts)
	}
}

func TestNoChangeObjectReplayStillRequiresCurrentWriteAuthorization(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	admin := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	ref := ordersReference(t, repo, "")
	body(t, admin("writer", "put", ref, "--repo", repo, "--command-id", "no-change-auth-seed", "--value", `1`))
	grant := asMap(t, body(t, admin("grant", "add", "--principal", "no-change-publisher", "--repo", repo, "--action", "writer.commit")))
	body(t, admin("grant", "add", "--principal", "no-change-publisher", "--repo", repo, "--action", "object.read"))
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "no-change-publisher", args...) }
	receipt := asMap(t, body(t, run("writer", "put", ref, "--repo", repo, "--command-id", "no-change-auth", "--value", `1`)))
	if asMap(t, receipt["result"])["changed"] != false {
		t.Fatalf("fixture did not accept no-change: %#v", receipt)
	}
	body(t, admin("grant", "remove", "--id", grant["id"].(string)))
	body(t, run("read", ref, "--repo", repo))
	expectCode(t, run("writer", "put", ref, "--repo", repo, "--command-id", "no-change-auth", "--value", `1`), string(kernel.ErrForbidden))
}

func TestReceiptOnlyGrantDoesNotExposeObjectRecoveryMaterial(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	admin := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	ref := ordersReference(t, repo, "")
	value := `{"body":"private-recovery-marker"}`
	body(t, admin("writer", "put", ref, "--repo", repo, "--command-id", "private-seed", "--value", value))
	body(t, admin("writer", "put", ref, "--repo", repo, "--command-id", "private-no-change", "--value", value))
	body(t, admin("grant", "add", "--principal", "receipt-only", "--repo", repo, "--action", "writer.receipt.read"))
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "receipt-only", args...) }
	expectCode(t, run("read", ref, "--repo", repo), string(kernel.ErrForbidden))
	lookup := asMap(t, body(t, run("writer", "receipt", "--command-id", "private-no-change")))
	if asMap(t, asMap(t, lookup["receipt"])["result"])["changed"] != false {
		t.Fatalf("missing no-change result: %#v", lookup)
	}
	request := asMap(t, lookup["request"])
	for _, key := range []string{"rawChangeSet", "applicationRequest", "authority"} {
		if _, ok := request[key]; ok {
			t.Fatalf("receipt exposes %s: %#v", key, lookup)
		}
	}
	for _, key := range []string{"execution", "phase"} {
		if _, ok := lookup[key]; ok {
			t.Fatalf("receipt exposes %s: %#v", key, lookup)
		}
	}
}

func TestDirectoryFullIntentNoChangeAndChangedRequest(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	dir := t.TempDir()
	path := filepath.Join(dir, "note.json")
	if err := os.WriteFile(path, []byte("---\nobject_id: note/directory\n---\n{\"body\":\"same\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	commit := func(id string) kcRunResult {
		return run("writer", "commit", "--repo", repo, "--dir", dir, "--command-id", id)
	}
	seed := asMap(t, body(t, commit("directory-seed")))
	first := asMap(t, body(t, commit("directory-no-change")))
	result := asMap(t, first["result"])
	if result["changed"] != false || result["newCommit"] != asMap(t, seed["result"])["newCommit"] {
		t.Fatalf("directory no-change lost basis: %#v", first)
	}
	// An unrelated publisher cannot change the basis or result of a completed command.
	ref := ordersReference(t, repo, "")
	body(t, run("writer", "put", ref, "--repo", repo, "--command-id", "directory-unrelated", "--value", `1`))
	replay := asMap(t, body(t, commit("directory-no-change")))
	if replay["disposition"] != "REPLAYED" || asMap(t, replay["result"])["newCommit"] != result["newCommit"] {
		t.Fatalf("directory retry adopted a new basis: %#v", replay)
	}
	if err := os.WriteFile(path, []byte("---\nobject_id: note/directory\npath_hint: moved/note.json\n---\n{\"body\":\"same\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	expectCode(t, commit("directory-no-change"), string(kernel.ErrIdempotencyConflict))
	changed := asMap(t, body(t, commit("directory-relocate")))
	if asMap(t, changed["result"])["changed"] != true {
		t.Fatalf("directory discarded path effect: %#v", changed)
	}
}
