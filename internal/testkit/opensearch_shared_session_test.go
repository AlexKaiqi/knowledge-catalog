package testkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// fakeProjectionCluster records the indices a run creates and the deletes the
// reclaim issues, so the session rule can be tested without an instance.
type fakeProjectionCluster struct {
	mu      sync.Mutex
	indices map[string]bool
	deleted []string
}

func (c *fakeProjectionCluster) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/_cat/indices/"):
			names := make([]string, 0, len(c.indices))
			for name := range c.indices {
				names = append(names, name)
			}
			sort.Strings(names)
			rows := make([]map[string]string, 0, len(names))
			for _, name := range names {
				rows = append(rows, map[string]string{"index": name})
			}
			_ = json.NewEncoder(w).Encode(rows)
		case r.Method == http.MethodDelete:
			name := strings.TrimPrefix(r.URL.Path, "/")
			delete(c.indices, name)
			c.deleted = append(c.deleted, name)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
}

func (c *fakeProjectionCluster) add(names ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, name := range names {
		c.indices[name] = true
	}
}

func (c *fakeProjectionCluster) remaining() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.indices))
	for name := range c.indices {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *fakeProjectionCluster) removals() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.deleted...)
}

func newFakeCluster(t *testing.T) (*fakeProjectionCluster, *httptest.Server) {
	t.Helper()
	cluster := &fakeProjectionCluster{indices: map[string]bool{}}
	server := httptest.NewServer(cluster.handler())
	t.Cleanup(server.Close)
	t.Setenv(sharedSearchSessionDirEnv, t.TempDir())
	return cluster, server
}

// Packages run in parallel against one instance, so a reclaim that fires while a
// sibling still has homes open deletes indices that are being asserted on. The
// first member out therefore reclaims nothing, and the last one reclaims the
// whole run's difference in one pass.
func TestSharedSearchSessionReclaimsOnlyWhenTheLastMemberLeaves(t *testing.T) {
	cluster, server := newFakeCluster(t)
	cluster.add("kc-proj-existing-g-one", "kc-projection-control-v1")

	first, err := joinSharedSearchSession(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	second, err := joinSharedSearchSession(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster.add("kc-proj-first-g-two", "kc-proj-second-g-three", "kc-projection-control-v1-newscan")

	if err := first.reclaim(); err != nil {
		t.Fatalf("first member reclaim: %v", err)
	}
	if removals := cluster.removals(); len(removals) != 0 {
		t.Fatalf("a member left while a sibling was still running and deleted %v", removals)
	}
	if err := second.reclaim(); err != nil {
		t.Fatalf("last member reclaim: %v", err)
	}

	remaining := cluster.remaining()
	want := []string{"kc-proj-existing-g-one", "kc-projection-control-v1"}
	if strings.Join(remaining, ",") != strings.Join(want, ",") {
		t.Fatalf("reclaim left %v, want the pre-existing indices %v", remaining, want)
	}
	removals := cluster.removals()
	sort.Strings(removals)
	expected := []string{"kc-proj-first-g-two", "kc-proj-second-g-three", "kc-projection-control-v1-newscan"}
	if strings.Join(removals, ",") != strings.Join(expected, ",") {
		t.Fatalf("reclaim removed %v, want exactly %v", removals, expected)
	}
}

// A run killed mid-flight leaves a lease file but no lock, because the kernel
// released it. Treating that file as a live member would make every later run
// wait for a process that no longer exists, so it is reaped.
func TestSharedSearchSessionReapsALeaseWhoseOwnerIsGone(t *testing.T) {
	cluster, server := newFakeCluster(t)
	dir := os.Getenv(sharedSearchSessionDirEnv)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sharedSearchMemberPrefix+"crashed"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	session, err := joinSharedSearchSession(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster.add("kc-proj-after-crash-g-one")
	if err := session.reclaim(); err != nil {
		t.Fatalf("reclaim after a crashed member: %v", err)
	}
	if remaining := cluster.remaining(); len(remaining) != 0 {
		t.Fatalf("a stale lease held the reclaim back: %v", remaining)
	}
}

// The opening inventory belongs to the session, not to the member that happened
// to take it: a later member must not adopt the run's earlier indices as
// pre-existing, which would leave them behind.
func TestSharedSearchSessionSharesOneOpeningInventory(t *testing.T) {
	cluster, server := newFakeCluster(t)
	cluster.add("kc-proj-existing-g-one")

	first, err := joinSharedSearchSession(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster.add("kc-proj-between-members-g-two")
	second, err := joinSharedSearchSession(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cluster.add("kc-proj-late-g-three")
	if err := first.reclaim(); err != nil {
		t.Fatal(err)
	}
	if err := second.reclaim(); err != nil {
		t.Fatal(err)
	}

	remaining := cluster.remaining()
	want := []string{"kc-proj-existing-g-one"}
	if strings.Join(remaining, ",") != strings.Join(want, ",") {
		t.Fatalf("reclaim left %v, want only the pre-existing index %v", remaining, want)
	}
}

// A session whose reclaim never ran leaves its opening inventory behind. The
// next session inherits it instead of retaking one: an index created by the
// abandoned run is still ours, and retaking the inventory would adopt it.
func TestSharedSearchSessionInheritsAnAbandonedOpeningInventory(t *testing.T) {
	cluster, server := newFakeCluster(t)
	cluster.add("kc-proj-existing-g-one")

	abandoned, err := joinSharedSearchSession(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	abandoned.member.release()
	cluster.add("kc-proj-from-abandoned-run-g-two")

	next, err := joinSharedSearchSession(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := next.reclaim(); err != nil {
		t.Fatal(err)
	}

	remaining := cluster.remaining()
	want := []string{"kc-proj-existing-g-one"}
	if strings.Join(remaining, ",") != strings.Join(want, ",") {
		t.Fatalf("reclaim left %v, want only the pre-existing index %v", remaining, want)
	}
}

// A process that was re-executed by a test inherits ownership instead of
// joining: it is a child of a run whose member is still alive, and its own
// membership would keep the reclaim from ever running.
func TestOwnSharedSearchIndicesIsNotJoinedTwiceByAChildProcess(t *testing.T) {
	_, server := newFakeCluster(t)
	t.Setenv("KC_TEST_OPENSEARCH_URL", server.URL)
	t.Setenv(sharedSearchOwnerEnv, "inherited-run")

	if reclaim := OwnSharedSearchIndices(); reclaim != nil {
		t.Fatal("a child of an owning run must not join the session again")
	}
}

func TestOwnSharedSearchIndicesRequiresAnEndpoint(t *testing.T) {
	t.Setenv("KC_TEST_OPENSEARCH_URL", "")
	t.Setenv(sharedSearchSessionDirEnv, t.TempDir())
	if reclaim := OwnSharedSearchIndices(); reclaim != nil {
		t.Fatal("an unbound run has nothing to reclaim")
	}
}

// The exported entry is what the package entry points use, so it has to leave
// the run marked while it is a member and clear the mark when it reclaims.
func TestOwnSharedSearchIndicesMarksTheRunAndClearsItOnReclaim(t *testing.T) {
	cluster, server := newFakeCluster(t)
	cluster.add("kc-proj-existing-g-one")
	t.Setenv("KC_TEST_OPENSEARCH_URL", server.URL)
	_ = os.Unsetenv(sharedSearchOwnerEnv)

	reclaim := OwnSharedSearchIndices()
	if reclaim == nil {
		t.Fatal("a bound endpoint must join the run's session")
	}
	if os.Getenv(sharedSearchOwnerEnv) == "" {
		t.Fatal("the joining process must mark the run so its children do not join")
	}
	cluster.add("kc-proj-this-run-g-two")
	if err := reclaim(); err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if os.Getenv(sharedSearchOwnerEnv) != "" {
		t.Fatal("the run mark survived the reclaim")
	}
	if remaining := cluster.remaining(); strings.Join(remaining, ",") != "kc-proj-existing-g-one" {
		t.Fatalf("reclaim left %v, want only the pre-existing index", remaining)
	}
}
