package snapshot_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

// W-02: losing the verification response after target publication must not
// release the command's digest claim, including after opening a new ledger.
func TestRawWriteLostPublicationAcknowledgementKeepsPendingClaim(t *testing.T) {
	fake := newFakeLakeFS(t)
	var failVerification atomic.Bool
	var injectEnabled atomic.Bool
	injectEnabled.Store(true)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/branches/main") && failVerification.Swap(false) {
			http.Error(w, "lost post-reset acknowledgement", http.StatusServiceUnavailable)
			return
		}
		fake.serveHTTP(w, r)
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/branches/main/hard_reset") && injectEnabled.Swap(false) {
			failVerification.Store(true)
		}
	}))
	t.Cleanup(proxy.Close)
	fake.server = proxy
	repo := fake.open(t, "kr://conformance/uncertain-command")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	registry := snapshot.NewRegistry()
	if err := registry.Add(repo); err != nil {
		t.Fatal(err)
	}
	store := snapshot.NewBoltCommandStore(filepath.Join(t.TempDir(), "writer.db"))
	ledger, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := snapshot.NewRawWriter(registry, ledger)
	if err != nil {
		t.Fatal(err)
	}
	cs := snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root, RequestID: "uncertain-command",
		Changes: []snapshot.TreeChange{{Path: "file.txt", Content: []byte("first")}},
	}
	receipt, err := writer.Commit("uncertain-command", cs)
	testkit.ExpectCode(t, err, kernel.ErrTemporaryUnavailable)
	if receipt.Result.NewCommit != "" {
		t.Fatalf("unknown publication returned a successful receipt: %+v", receipt)
	}
	head := testkit.MustHead(t, repo, snapshot.DefaultRef)
	if head == root {
		t.Fatal("fault injection did not publish before losing verification")
	}
	if body, err := repo.ReadFile("file.txt", head); err != nil || string(body) != "first" {
		t.Fatalf("published body = %q, error = %v", body, err)
	}

	restarted, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	for name, commands := range map[string]snapshot.CommandLedger{"current": ledger, "restarted": restarted} {
		entry, ok := commands.Lookup("uncertain-command")
		if !ok || entry.Status != snapshot.StatusPending || entry.Digest != string(kernel.CanonicalDigest(cs)) || (len(entry.Receipt) != 0 && string(entry.Receipt) != "null") {
			t.Errorf("%s ledger lost unresolved digest claim: %+v, found = %v", name, entry, ok)
		}
	}
	writer, err = snapshot.NewRawWriter(registry, restarted)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := writer.Lookup("uncertain-command"); ok {
		t.Error("unresolved command was exposed as an accepted raw receipt")
	}
	_, err = writer.Commit("uncertain-command", cs)
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Errorf("same payload must not reapply unresolved command: %v", err)
	}
	different := cs
	different.BaseCommit, different.ExpectedTargetCommit = head, head
	different.Changes = []snapshot.TreeChange{{Path: "file.txt", Content: []byte("second")}}
	_, err = writer.Commit("uncertain-command", different)
	if kernel.CodeOf(err) != kernel.ErrIdempotencyConflict {
		t.Errorf("different payload reused unresolved command: %v", err)
	}
	if final := testkit.MustHead(t, repo, snapshot.DefaultRef); final != head {
		t.Errorf("retry changed published head: %s → %s", head, final)
	}
}

func TestRawWritePrePublicationFailureReleasesClaim(t *testing.T) {
	fake := newFakeLakeFS(t)
	var failStaging atomic.Bool
	failStaging.Store(true)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/staging/backing") && failStaging.Swap(false) {
			http.Error(w, "staging unavailable before publication", http.StatusServiceUnavailable)
			return
		}
		fake.serveHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	fake.server = proxy
	repo := fake.open(t, "kr://conformance/unapplied-command")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	registry := snapshot.NewRegistry()
	if err := registry.Add(repo); err != nil {
		t.Fatal(err)
	}
	store := snapshot.NewBoltCommandStore(filepath.Join(t.TempDir(), "writer.db"))
	ledger, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := snapshot.NewRawWriter(registry, ledger)
	if err != nil {
		t.Fatal(err)
	}
	cs := snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []snapshot.TreeChange{{Path: "file.txt", Content: []byte("retry")}},
	}
	_, err = writer.Commit("unapplied", cs)
	testkit.ExpectCode(t, err, kernel.ErrTemporaryUnavailable)
	if testkit.MustHead(t, repo, snapshot.DefaultRef) != root {
		t.Fatal("pre-publication error moved the authoritative target")
	}
	if _, ok := ledger.Lookup("unapplied"); ok {
		t.Fatal("proven unapplied error retained in-memory claim")
	}
	restarted, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := restarted.Lookup("unapplied"); ok {
		t.Fatal("proven unapplied error retained durable claim")
	}
	writer, err = snapshot.NewRawWriter(registry, restarted)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := writer.Commit("unapplied", cs)
	if err != nil || receipt.Disposition != snapshot.ReceiptApplied || receipt.Result.NewCommit == root {
		t.Fatalf("proven unapplied command could not retry: %+v, %v", receipt, err)
	}
	if body, err := repo.ReadFile("file.txt", receipt.Result.NewCommit); err != nil || string(body) != "retry" {
		t.Fatalf("retry published body = %q, error = %v", body, err)
	}
}
