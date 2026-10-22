package testfixture

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// fakeCluster is a minimal OpenSearch stand-in. It reproduces the engine's
// refusal to restore onto an existing index name, which is why the fixture must
// delete the target first instead of relying on an overwrite.
type fakeCluster struct {
	mu            sync.Mutex
	indices       map[string][]string
	snapshots     map[string]map[string][]string
	registered    bool
	shardLimit    int
	rejectRestore bool
	// sources is the identity stamped on every seeded document.
	sources []SourceRef
}

func newFakeCluster(t *testing.T) (*fakeCluster, *SnapshotCheckpoint) {
	t.Helper()
	cluster := &fakeCluster{
		indices:       map[string][]string{},
		snapshots:     map[string]map[string][]string{},
		registered:    true,
		shardLimit:    1000,
		rejectRestore: true,
		sources:       []SourceRef{{Repository: "scnlcapture01", Commit: "commit-capture"}},
	}
	server := httptest.NewServer(cluster)
	t.Cleanup(server.Close)
	checkpoint, err := NewSnapshotCheckpoint(server.URL, "kcrepo")
	if err != nil {
		t.Fatal(err)
	}
	return cluster, checkpoint
}

func (c *fakeCluster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/")
	switch {
	case path == "_snapshot/kcrepo":
		if !c.registered {
			http.Error(w, `{"error":"repository_missing"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"kcrepo": map[string]any{"type": "fs"}})
	case strings.HasPrefix(path, "_snapshot/kcrepo/"):
		c.handleSnapshot(w, r, strings.TrimPrefix(path, "_snapshot/kcrepo/"))
	case path == "_cluster/settings":
		writeJSON(w, map[string]any{"defaults": map[string]any{
			"cluster.max_shards_per_node": fmt.Sprint(c.shardLimit),
		}})
	case path == "_all/_settings":
		writeJSON(w, c.settingsFor(c.allNames()))
	case strings.HasSuffix(path, "/_settings"):
		writeJSON(w, c.settingsFor(c.matchIndices(strings.TrimSuffix(path, "/_settings"))))
	case strings.HasSuffix(path, "/_refresh"):
		writeJSON(w, map[string]any{"_shards": map[string]any{"failed": 0}})
	case strings.HasSuffix(path, "/_search") && r.Method == http.MethodPost:
		buckets := []map[string]any{}
		for _, ref := range c.sources {
			// multi_terms keys are arrays in field order; mirroring that keeps the fake
			// from accepting a shape a real cluster never returns.
			buckets = append(buckets, map[string]any{"key": []string{ref.Repository, ref.Commit}})
		}
		writeJSON(w, map[string]any{"aggregations": map[string]any{"pairs": map[string]any{"buckets": buckets}}})
	case strings.HasPrefix(path, "_cat/indices/"):
		c.handleCatIndices(w, r)
	case r.Method == http.MethodDelete:
		name := strings.TrimPrefix(path, "")
		if _, ok := c.indices[name]; !ok {
			http.Error(w, `{"error":"index_not_found"}`, http.StatusNotFound)
			return
		}
		delete(c.indices, name)
		writeJSON(w, map[string]any{"acknowledged": true})
	default:
		http.Error(w, `{"error":"unexpected "+r.Method+" "+r.URL.Path+"'}`, http.StatusBadRequest)
	}
}

func (c *fakeCluster) handleSnapshot(w http.ResponseWriter, r *http.Request, rest string) {
	switch {
	case r.Method == http.MethodPut:
		name := rest
		var body struct {
			Indices string `json:"indices"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured := map[string][]string{}
		for _, index := range strings.Split(body.Indices, ",") {
			if documents, ok := c.indices[index]; ok {
				captured[index] = append([]string(nil), documents...)
			}
		}
		if len(captured) == 0 {
			http.Error(w, `{"error":"no indices"}`, http.StatusBadRequest)
			return
		}
		c.snapshots[name] = captured
		writeJSON(w, map[string]any{"snapshot": map[string]any{
			"state":  "SUCCESS",
			"shards": map[string]any{"failed": 0},
		}})
	case strings.HasSuffix(rest, "/_restore"):
		name := strings.TrimSuffix(rest, "/_restore")
		captured, ok := c.snapshots[name]
		if !ok {
			http.Error(w, `{"error":"snapshot_missing"}`, http.StatusNotFound)
			return
		}
		var body struct {
			RenamePattern     string `json:"rename_pattern"`
			RenameReplacement string `json:"rename_replacement"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		pattern := regexp.MustCompile(body.RenamePattern)
		for index, documents := range captured {
			target := index
			if pattern.MatchString(index) {
				target = pattern.ReplaceAllString(index, body.RenameReplacement)
			}
			if _, exists := c.indices[target]; exists && c.rejectRestore {
				// The engine refuses the whole request instead of overwriting.
				http.Error(w, `{"error":{"type":"snapshot_restore_exception","reason":"cannot restore index [`+target+`] because an open index with same name already exists"}}`, http.StatusInternalServerError)
				return
			}
			c.indices[target] = append([]string(nil), documents...)
		}
		writeJSON(w, map[string]any{"snapshot": map[string]any{"shards": map[string]any{"failed": 0}}})
	default:
		http.Error(w, `{"error":"unsupported snapshot call"}`, http.StatusBadRequest)
	}
}

func (c *fakeCluster) handleCatIndices(w http.ResponseWriter, r *http.Request) {
	selector := strings.TrimPrefix(r.URL.Path, "/_cat/indices/")
	matched := c.matchIndices(selector)
	rows := make([]map[string]string, 0, len(matched))
	for _, index := range matched {
		rows = append(rows, map[string]string{
			"index":      index,
			"docs.count": fmt.Sprint(len(c.indices[index])),
		})
	}
	writeJSON(w, rows)
}

func (c *fakeCluster) matchIndices(selector string) []string {
	names := c.allNames()
	if len(selector) == 0 || selector == "*" {
		return names
	}
	if strings.Contains(selector, ",") {
		var matched []string
		for _, part := range strings.Split(selector, ",") {
			matched = append(matched, c.matchIndices(part)...)
		}
		return matched
	}
	if !strings.HasSuffix(selector, "*") {
		for _, name := range names {
			if name == selector {
				return []string{name}
			}
		}
		return nil
	}
	prefix := strings.TrimSuffix(selector, "*")
	var matched []string
	for _, name := range names {
		if strings.HasPrefix(name, prefix) {
			matched = append(matched, name)
		}
	}
	return matched
}

func (c *fakeCluster) allNames() []string {
	names := make([]string, 0, len(c.indices))
	for name := range c.indices {
		names = append(names, name)
	}
	return names
}

func (c *fakeCluster) settingsFor(names []string) map[string]any {
	settings := map[string]any{}
	for _, name := range names {
		settings[name] = map[string]any{"settings": map[string]any{
			"index.number_of_shards":   "2",
			"index.number_of_replicas": "0",
		}}
	}
	return settings
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

// seedSource writes the capture-time state: two owned indices, three documents.
func seedSource(t *testing.T, cluster *fakeCluster, namespace string) {
	t.Helper()
	prefix := ProjectionPrefix(namespace)
	if prefix == "" {
		t.Fatal("seed requires an owned namespace")
	}
	cluster.mu.Lock()
	defer cluster.mu.Unlock()
	cluster.indices[prefix+"objects"] = []string{"a", "b"}
	cluster.indices[prefix+"relations"] = []string{"r1"}
}

// TestRestoreReplacesContaminatedTarget is the behaviour this fixture exists for.
// The engine refuses to restore onto an existing index name, so the fixture has to
// delete the exact target first; the assertion is that the polluted documents are
// gone afterwards, which only the delete-then-verify sequence can guarantee.
func TestRestoreReplacesContaminatedTarget(t *testing.T) {
	cluster, checkpoint := newFakeCluster(t)
	seedSource(t, cluster, "capture")
	captured, err := checkpoint.Capture("kc-cp-base", "fp-1", "capture")
	if err != nil {
		t.Fatal(err)
	}
	if captured.Shards != 4 {
		t.Fatalf("captured shards = %d, want 4", captured.Shards)
	}
	if captured.Documents[ProjectionPrefix("capture")+"objects"] != 2 {
		t.Fatalf("captured documents = %v", captured.Documents)
	}

	// A polluted target must be replaced, not left behind.
	target := ProjectionPrefix("reuse")
	cluster.mu.Lock()
	cluster.indices[target+"objects"] = []string{"stale", "stale", "stale", "stale", "stale"}
	cluster.mu.Unlock()

	if err := checkpoint.Restore(captured, "fp-1", captured.Sources, "reuse"); err != nil {
		t.Fatalf("restore rejected a polluted target: %v", err)
	}
	cluster.mu.Lock()
	documents := cluster.indices[target+"objects"]
	cluster.mu.Unlock()
	if len(documents) != 2 || documents[0] != "a" || documents[1] != "b" {
		t.Fatalf("restore kept contaminated documents: %v", documents)
	}
}

// TestVerifyRejectsDocumentCountDrift proves the read-back comparison is the only
// signal that separates a real restore from a partial one: the engine's shard
// counter reports success either way.
func TestVerifyRejectsDocumentCountDrift(t *testing.T) {
	cluster, checkpoint := newFakeCluster(t)
	seedSource(t, cluster, "capture")
	captured, err := checkpoint.Capture("kc-cp-base", "fp-1", "capture")
	if err != nil {
		t.Fatal(err)
	}
	// Delete the target so the remaining signal is the document comparison alone.
	target := ProjectionPrefix("reuse")
	cluster.mu.Lock()
	cluster.indices[target+"objects"] = []string{"stale"}
	cluster.mu.Unlock()
	if err := checkpoint.dropOwned(target); err != nil {
		t.Fatal(err)
	}
	captured.Documents[ProjectionPrefix("capture")+"objects"] = 99
	if err := checkpoint.verify(captured, target); err == nil {
		t.Fatal("verify accepted a document count that differs from the capture")
	}
}

// TestRestoreRejectsChangedFingerprint keeps a stale checkpoint from being reused
// after the scenario that produced it changed.
func TestRestoreRejectsChangedFingerprint(t *testing.T) {
	cluster, checkpoint := newFakeCluster(t)
	seedSource(t, cluster, "capture")
	captured, err := checkpoint.Capture("kc-cp-base", "fp-1", "capture")
	if err != nil {
		t.Fatal(err)
	}
	err = checkpoint.Restore(captured, "fp-2", captured.Sources, "reuse")
	if err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("changed fingerprint accepted: %v", err)
	}
}

// TestCaptureRequiresFingerprintAndOwnedNamespace rejects checkpoints that could
// not be validated later.
func TestCaptureRequiresFingerprintAndOwnedNamespace(t *testing.T) {
	cluster, checkpoint := newFakeCluster(t)
	seedSource(t, cluster, "capture")
	if _, err := checkpoint.Capture("kc-cp-base", "", "capture"); err == nil {
		t.Fatal("capture accepted an empty fingerprint")
	}
	if _, err := checkpoint.Capture("kc-cp-base", "fp-1", ""); err == nil {
		t.Fatal("capture accepted an empty namespace")
	}
	if _, err := checkpoint.Capture("bad name", "fp-1", "capture"); err == nil {
		t.Fatal("capture accepted an invalid checkpoint name")
	}
}

// TestRestoreRequiresDistinctNamespace refuses to restore over the capture itself,
// which would destroy the reusable starting point.
func TestRestoreRequiresDistinctNamespace(t *testing.T) {
	cluster, checkpoint := newFakeCluster(t)
	seedSource(t, cluster, "capture")
	captured, err := checkpoint.Capture("kc-cp-base", "fp-1", "capture")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.Restore(captured, "fp-1", captured.Sources, "capture"); err == nil {
		t.Fatal("restore accepted the capture namespace as its target")
	}
}

// TestRestoreRejectsEmptyNamespace makes the boundary explicit: a namespace is
// what proves ownership, so an empty one has no prefix to delete or verify.
func TestRestoreRejectsEmptyNamespace(t *testing.T) {
	cluster, checkpoint := newFakeCluster(t)
	seedSource(t, cluster, "capture")
	captured, err := checkpoint.Capture("kc-cp-base", "fp-1", "capture")
	if err != nil {
		t.Fatal(err)
	}
	captured.Namespace = ""
	if err := checkpoint.Restore(captured, "fp-1", captured.Sources, "reuse"); err == nil {
		t.Fatal("restore accepted an empty captured namespace")
	}
	if err := checkpoint.dropOwned("kc-proj-nothex-"); err == nil {
		t.Fatal("deletion accepted a prefix without an owned scope")
	}
	if err := checkpoint.dropOwned(""); err == nil {
		t.Fatal("deletion accepted an empty prefix")
	}
}

// TestReserveRejectsOverLimit turns a shard exhaustion into a diagnosis here
// instead of a partially attempted restore inside the engine.
func TestReserveRejectsOverLimit(t *testing.T) {
	cluster, checkpoint := newFakeCluster(t)
	cluster.mu.Lock()
	cluster.shardLimit = 4
	cluster.indices[ProjectionPrefix("occupied")+"objects"] = []string{"x"}
	cluster.mu.Unlock()
	err := checkpoint.reserve(4)
	if err == nil || !strings.Contains(err.Error(), "open shards") {
		t.Fatalf("reserve accepted a restore over the shard limit: %v", err)
	}
}

// TestReleaseRemovesCapturedIndices keeps cleanup tied to the recorded prefix, so
// a cancelled run does not leave owned indices behind.
func TestReleaseRemovesCapturedIndices(t *testing.T) {
	cluster, checkpoint := newFakeCluster(t)
	seedSource(t, cluster, "capture")
	captured, err := checkpoint.Capture("kc-cp-base", "fp-1", "capture")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.Release(captured); err != nil {
		t.Fatal(err)
	}
	cluster.mu.Lock()
	remaining := len(cluster.indices)
	cluster.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("release left %d owned indices", remaining)
	}
	captured.Namespace = ""
	if err := checkpoint.Release(captured); err == nil {
		t.Fatal("release accepted an empty namespace")
	}
}

// TestRestoreRejectsForeignSources is the constraint measured against lakeFS: a
// commit ID hashes its creation timestamp and parent list, so two runs with
// identical content still produce different IDs, and a rebuilt repository leaves
// the old commit unresolvable (410). Restoring run A's documents into run B leaves
// a hit's source_commit pointing at unreadable data while counts still match.
func TestRestoreRejectsForeignSources(t *testing.T) {
	cluster, checkpoint := newFakeCluster(t)
	seedSource(t, cluster, "capture")
	captured, err := checkpoint.Capture("kc-cp-base", "fp-1", "capture")
	if err != nil {
		t.Fatal(err)
	}
	if len(captured.Sources) == 0 {
		t.Fatal("capture did not record any source identity")
	}
	foreign := []SourceRef{{Repository: "scnlsomeoneelse", Commit: captured.Sources[0].Commit}}
	err = checkpoint.Restore(captured, "fp-1", foreign, "reuse")
	if err == nil || !strings.Contains(err.Error(), "was captured from") {
		t.Fatalf("restore accepted a checkpoint from another run's sources: %v", err)
	}
	if err := checkpoint.Restore(captured, "fp-1", captured.Sources, "reuse"); err != nil {
		t.Fatalf("restore rejected the matching sources: %v", err)
	}
}

// TestCaptureRejectsMissingSources refuses a capture whose documents carry no
// source identity: nothing can then prove the documents still resolve.
func TestCaptureRejectsMissingSources(t *testing.T) {
	cluster, checkpoint := newFakeCluster(t)
	cluster.mu.Lock()
	cluster.sources = nil
	cluster.mu.Unlock()
	seedSource(t, cluster, "capture")
	if _, err := checkpoint.Capture("kc-cp-base", "fp-1", "capture"); err == nil {
		t.Fatal("capture accepted documents with no source identity")
	}
}

// TestCaptureRecordsEverySource keeps a multi-repository capture honest: a Dataset
// projection reads across source repositories, so more than one identity is normal
// and every one of them must be recorded. Dropping any would let a restore hand
// back candidates for a source the capture never proved readable.
func TestCaptureRecordsEverySource(t *testing.T) {
	cluster, checkpoint := newFakeCluster(t)
	cluster.mu.Lock()
	cluster.sources = []SourceRef{
		{Repository: "scnlaaa111", Commit: "commit-one"},
		{Repository: "scnlbbb222", Commit: "commit-two"},
	}
	cluster.mu.Unlock()
	seedSource(t, cluster, "capture")
	captured, err := checkpoint.Capture("kc-cp-base", "fp-1", "capture")
	if err != nil {
		t.Fatal(err)
	}
	if len(captured.Sources) != 2 {
		t.Fatalf("captured sources = %v, want both repositories", captured.Sources)
	}
	// Order must not decide equality.
	reversed := []SourceRef{captured.Sources[1], captured.Sources[0]}
	if err := checkpoint.Restore(captured, "fp-1", reversed, "reuse"); err != nil {
		t.Fatalf("restore rejected the same sources in another order: %v", err)
	}
	// A subset is a different capture: the missing repository is unproven.
	if err := checkpoint.Restore(captured, "fp-1", captured.Sources[:1], "reuse"); err == nil {
		t.Fatal("restore accepted a subset of the captured sources")
	}
}

// TestNewSnapshotCheckpointRequiresRegisteredRepository fails closed: a cluster
// without a snapshot repository must not look usable.
func TestNewSnapshotCheckpointRequiresRegisteredRepository(t *testing.T) {
	cluster, _ := newFakeCluster(t)
	cluster.mu.Lock()
	cluster.registered = false
	cluster.mu.Unlock()
	server := httptest.NewServer(cluster)
	t.Cleanup(server.Close)
	if _, err := NewSnapshotCheckpoint(server.URL, "kcrepo"); err == nil {
		t.Fatal("checkpoint bound an unregistered repository")
	}
	if _, err := NewSnapshotCheckpoint("", "kcrepo"); err == nil {
		t.Fatal("checkpoint bound an empty endpoint")
	}
	if _, err := NewSnapshotCheckpoint(server.URL, "Bad Name"); err == nil {
		t.Fatal("checkpoint bound an invalid repository name")
	}
}
