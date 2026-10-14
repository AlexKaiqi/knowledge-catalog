package snapshot_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

func TestLakeFSPublicationLockRecoveryRequiresObservedCoordinates(t *testing.T) {
	fake := newFakeLakeFS(t)
	repo := fake.open(t, "kr://conformance/interrupted-publisher")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	lock := "kc-publish-lock-" + string(kernel.CanonicalDigest(string(repo.ID()) + "\x00main"))[:24]
	fake.mu.Lock()
	fake.commits["orphan"] = fakeCommit{parent: string(root), files: map[string][]byte{"unpublished": []byte("candidate")}}
	fake.branches[lock] = "orphan"
	fake.mu.Unlock()
	repo = fake.open(t, string(repo.ID()))
	cs := snapshot.TreeChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root, Changes: []snapshot.TreeChange{{Path: "accepted", Content: []byte("after recovery")}}}
	_, err := repo.ApplyTreeCommit(cs)
	testkit.ExpectCode(t, err, kernel.ErrTemporaryUnavailable)
	for _, evidence := range []string{string(root), "orphan", "RecoverLakeFSPublication"} {
		if !strings.Contains(err.Error(), evidence) {
			t.Errorf("held-lock error omits recovery evidence %q: %v", evidence, err)
		}
	}
	if testkit.MustHead(t, repo, snapshot.DefaultRef) != root {
		t.Fatal("blocked publisher advanced target")
	}
	for _, row := range []struct {
		target, candidate kernel.CommitID
		code              kernel.ErrorCode
	}{{"stale", "orphan", kernel.ErrNonFastForward}, {root, "other", kernel.ErrPreconditionFailed}} {
		testkit.ExpectCode(t, snapshot.RecoverLakeFSPublication(repo, snapshot.DefaultRef, row.target, row.candidate), row.code)
		fake.mu.Lock()
		retained := fake.branches[lock]
		fake.mu.Unlock()
		if retained != "orphan" {
			t.Fatal("stale recovery observation deleted the lock")
		}
	}
	for range 2 {
		if err := snapshot.RecoverLakeFSPublication(repo, snapshot.DefaultRef, root, "orphan"); err != nil {
			t.Fatal(err)
		}
		if testkit.MustHead(t, repo, snapshot.DefaultRef) != root {
			t.Fatal("recovery advanced the publication target")
		}
	}
	accepted, err := repo.ApplyTreeCommit(cs)
	if err != nil {
		t.Fatal(err)
	}
	if body, err := repo.ReadFile("accepted", accepted); err != nil || string(body) != "after recovery" {
		t.Fatalf("recovered publication %q: %v", body, err)
	}
}

func TestLakeFSPublicationRecoveryFailurePreservesLockAndTarget(t *testing.T) {
	fake := newFakeLakeFS(t)
	var refuseDelete atomic.Bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/branches/kc-publish-lock-") && refuseDelete.Load() {
			http.Error(w, "recovery storage unavailable", http.StatusServiceUnavailable)
			return
		}
		fake.serveHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	fake.server = proxy
	repo := fake.open(t, "kr://conformance/recovery-failure")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	lock := "kc-publish-lock-" + string(kernel.CanonicalDigest(string(repo.ID()) + "\x00main"))[:24]
	fake.mu.Lock()
	fake.branches[lock] = string(root)
	fake.mu.Unlock()
	refuseDelete.Store(true)
	testkit.ExpectCode(t, snapshot.RecoverLakeFSPublication(repo, snapshot.DefaultRef, root, root), kernel.ErrTemporaryUnavailable)
	fake.mu.Lock()
	retained := fake.branches[lock]
	fake.mu.Unlock()
	if retained != string(root) || testkit.MustHead(t, repo, snapshot.DefaultRef) != root {
		t.Fatal("failed recovery changed lock or target")
	}
	refuseDelete.Store(false)
	if err := snapshot.RecoverLakeFSPublication(repo, snapshot.DefaultRef, root, root); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"", "HEAD", "main", "refs/heads/"} {
		testkit.ExpectCode(t, snapshot.RecoverLakeFSPublication(repo, ref, root, root), kernel.ErrUsageInvalid)
	}
	testkit.ExpectCode(t, snapshot.RecoverLakeFSPublication(testkit.MakeRepository(t, "kr://conformance/non-lakefs"), snapshot.DefaultRef, root, root), kernel.ErrCapabilityUnsatisfied)
}
