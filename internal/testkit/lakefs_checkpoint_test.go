package testkit

import (
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func checkpointFixture(t *testing.T) (*LakeFSFake, *LakeFSLive, string, string) {
	t.Helper()
	fake := NewLakeFSFake(t)
	live := NewLakeFSLive(t, fake.Origin(), fake.Credential(), "s3://checkpoint-test")
	name := live.NewRepo()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "remote.yaml"), []byte("id: kr://checkpoint/test\ndriver: lakefs\ndsn: "+live.DSN(name)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return fake, live, name, home
}

func TestLakeFSCheckpointRestoresRefsMetadataAndUncommittedWrites(t *testing.T) {
	fake, live, name, home := checkpointFixture(t)
	fake.mu.Lock()
	repo := fake.repos[name]
	repo.branches["review/keep"] = "c0"
	repo.metadata["kept"] = "baseline"
	fake.mu.Unlock()
	restore, err := live.SnapshotStamps(home)
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	repo.commits["probe-commit"] = lakefsFakeCommit{parent: "c0", files: map[string][]byte{"changed": []byte("probe")}}
	repo.branches["main"] = "probe-commit"
	repo.branches["probe-only"] = "probe-commit"
	delete(repo.branches, "review/keep")
	data := []byte("uncommitted")
	repo.staged["main"] = map[string]*[]byte{"pending": &data}
	repo.metadata["kept"] = "changed"
	repo.metadata["extra"] = "probe"
	fake.mu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if err := restore(); err != nil {
			t.Fatal(err)
		}
		fake.mu.Lock()
		branches, metadata := maps.Clone(repo.branches), maps.Clone(repo.metadata)
		staged := len(repo.staged)
		_, historyKept := repo.commits["probe-commit"]
		fake.mu.Unlock()
		if !maps.Equal(branches, map[string]string{"main": "c0", "review/keep": "c0"}) || !maps.Equal(metadata, map[string]string{"kept": "baseline"}) || staged != 0 || !historyKept {
			t.Fatalf("checkpoint did not restore exact refs/properties while retaining immutable history: branches=%v metadata=%v staged=%d history=%t", branches, metadata, staged, historyKept)
		}
	}
}

func TestLakeFSCheckpointRejectsUnownedAuthority(t *testing.T) {
	_, live, _, home := checkpointFixture(t)
	path := filepath.Join(home, "catalog-authority.yaml")
	if err := os.WriteFile(path, []byte("id: kr://checkpoint/test\ndriver: lakefs\ndsn: "+live.DSN("not-owned")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := live.SnapshotStamps(home); err == nil || !strings.Contains(err.Error(), "unowned") {
		t.Fatalf("unowned authority checkpoint: %v", err)
	}
}

func TestLakeFSCheckpointRejectsChangedStorageIdentity(t *testing.T) {
	fake, live, name, home := checkpointFixture(t)
	restore, err := live.SnapshotStamps(home)
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.repos[name].storageNamespace = "s3://someone-else"
	fake.mu.Unlock()
	if err := restore(); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("replacement authority restored: %v", err)
	}
}

func TestLakeFSCheckpointFailsClosedOnBranchReadError(t *testing.T) {
	var fail atomic.Bool
	fake := NewLakeFSFakeWithInterceptor(t, func(w http.ResponseWriter, r *http.Request) bool {
		if fail.Load() && strings.HasSuffix(r.URL.Path, "/branches") && r.Method == http.MethodGet {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return true
		}
		return false
	})
	live := NewLakeFSLive(t, fake.Origin(), fake.Credential(), "s3://checkpoint-test")
	name := live.NewRepo()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "remote.yaml"), []byte("id: kr://checkpoint/test\ndriver: lakefs\ndsn: "+live.DSN(name)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restore, err := live.SnapshotStamps(home)
	if err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	if err := restore(); err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("failed branch inventory treated as empty: %v", err)
	}
}

func TestFakeCheckpointPreservesPhysicalAuthorityAndImmutableHistory(t *testing.T) {
	fake := NewLakeFSFake(t)
	name := fake.NewRepo()
	home := t.TempDir()
	stamp := []byte("id: kr://checkpoint/fake\ndriver: lakefs\ndsn: " + fake.DSN(name) + "\n")
	path := filepath.Join(home, "remote.yaml")
	if err := os.WriteFile(path, stamp, 0600); err != nil {
		t.Fatal(err)
	}
	restore, err := fake.SnapshotStamps(home)
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	repo := fake.repos[name]
	repo.commits["future"] = lakefsFakeCommit{parent: "c0", files: map[string][]byte{"proof": []byte("retained")}}
	repo.branches["main"] = "future"
	fake.mu.Unlock()
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != string(stamp) {
		t.Fatalf("authority stamp changed: %s %v", raw, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if repo.branches["main"] != "c0" || string(repo.commits["future"].files["proof"]) != "retained" {
		t.Fatal("checkpoint changed physical history instead of refs")
	}
}
