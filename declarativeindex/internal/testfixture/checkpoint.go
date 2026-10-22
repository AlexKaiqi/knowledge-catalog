package testfixture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"kc/kernel"
)

// ownedPrefix matches a projection namespace's physical index prefix. Every
// restore and delete in this file is limited to names carrying one of these.
var ownedPrefix = regexp.MustCompile(`^kc-proj-[0-9a-f]{24}-$`)

// checkpointName matches snapshot names this fixture accepts, so a restore can
// never address a snapshot written by another owner.
var checkpointName = regexp.MustCompile(`^kc-cp-[a-z0-9][a-z0-9-]{0,63}$`)

var repositoryName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// ProjectionPrefix returns the owned index prefix of a projection namespace.
// The empty namespace has no prefix: callers must allocate one per invocation.
func ProjectionPrefix(namespace string) string {
	if namespace == "" {
		return ""
	}
	return "kc-proj-" + projectionScope(namespace) + "-"
}

// projectionScope is the stable 24 hex character namespace scope. Cleanup and
// capture must derive it the same way or ownership cannot be proven.
func projectionScope(namespace string) string {
	return string(kernel.CanonicalDigest(namespace))[:24]
}

// ControlIndex returns the shared control index name owned by a namespace.
func ControlIndex(namespace string) string {
	return "kc-projection-control-v1-" + projectionScope(namespace)
}

// Checkpoint records one captured index set plus the evidence reuse depends on.
// Documents and Shards are the capture-time truth a restore is verified against;
// Fingerprint is what makes those documents still valid as a starting point.
type Checkpoint struct {
	Namespace   string
	Fingerprint string
	Snapshot    string
	Documents   map[string]int64
	Shards      int
	// Sources records the distinct (repository, commit) pairs stamped on the captured
	// documents. These are the commits of the repositories the projected objects were
	// read from, not the index basis: response.go hands a hit's source_commit back as
	// the candidate basis, so a document naming a source that no longer exists yields
	// a candidate pointing at unreadable data while every document count still matches.
	Sources []SourceRef
}

// SourceRef is one source repository at one commit, as stamped on a document.
type SourceRef struct {
	Repository string
	Commit     string
}

func (r SourceRef) String() string { return r.Repository + "@" + r.Commit }

// capturedSources reads the distinct source identities stamped on the captured
// documents. Unlike the index basis, this identity is produced by the repositories
// the objects were read from, so it cannot be reproduced by rebuilding a projection
// and has to be carried explicitly.
func (c *SnapshotCheckpoint) capturedSources(documents map[string]int64) ([]SourceRef, error) {
	seen := map[SourceRef]bool{}
	for index := range documents {
		var result struct {
			Aggregations struct {
				Pairs struct {
					Buckets []struct {
						// multi_terms returns the key as an array in field order, not as
						// an object; reading it as a struct fails to unmarshal on a real
						// cluster while a hand-written fake accepts it happily.
						Key []string `json:"key"`
					} `json:"buckets"`
				} `json:"pairs"`
			} `json:"aggregations"`
		}
		body := map[string]any{
			"size":             0,
			"track_total_hits": false,
			"aggs": map[string]any{"pairs": map[string]any{"multi_terms": map[string]any{
				"terms": []any{map[string]any{"field": "source_repository"}, map[string]any{"field": "source_commit"}},
				"size":  100,
			}}},
		}
		if err := c.do(http.MethodPost, "/"+url.PathEscape(index)+"/_search", body, &result); err != nil {
			return nil, fmt.Errorf("read source identity from %s: %w", index, err)
		}
		for _, bucket := range result.Aggregations.Pairs.Buckets {
			if len(bucket.Key) != 2 {
				return nil, fmt.Errorf("source identity in %s has %d key terms, want 2", index, len(bucket.Key))
			}
			if bucket.Key[0] != "" || bucket.Key[1] != "" {
				seen[SourceRef{Repository: bucket.Key[0], Commit: bucket.Key[1]}] = true
			}
		}
	}
	sources := make([]SourceRef, 0, len(seen))
	for ref := range seen {
		sources = append(sources, ref)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].String() < sources[j].String() })
	if len(sources) == 0 {
		return nil, fmt.Errorf("captured documents carry no source repository and commit")
	}
	return sources, nil
}

// sameSources reports whether two source sets name the same repositories at the
// same commits, ignoring order.
func sameSources(want, got []SourceRef) bool {
	if len(want) != len(got) {
		return false
	}
	index := make(map[SourceRef]bool, len(want))
	for _, ref := range want {
		index[ref] = true
	}
	for _, ref := range got {
		if !index[ref] {
			return false
		}
	}
	return true
}

func describeSources(sources []SourceRef) string {
	if len(sources) == 0 {
		return "no source"
	}
	parts := make([]string, 0, len(sources))
	for _, ref := range sources {
		parts = append(parts, ref.String())
	}
	return strings.Join(parts, ", ")
}

// SnapshotCheckpoint captures reusable index state through the cluster snapshot
// repository and restores it into a caller-owned namespace.
//
// Restore is guarded because the engine offers no safe reuse path. Preserving the
// original index name, or renaming onto an existing one, both fail the whole
// request with resource_already_exists / snapshot_restore_exception. Neither can
// be satisfied by retrying, and a successful shards counter alone never proves the
// target started clean. Restore therefore deletes the exact target indices first,
// confirms they are gone, then compares document counts with the capture, so a
// skipped or partial restore fails loudly instead of handing tests contaminated
// state that looks healthy.
type SnapshotCheckpoint struct {
	endpoint   string
	repository string
	client     *http.Client
}

// NewSnapshotCheckpoint binds an existing snapshot repository. The fixture never
// starts a service and never registers the repository itself: a cluster without
// path.repo must fail here rather than silently lose snapshots.
func NewSnapshotCheckpoint(endpoint, repository string) (*SnapshotCheckpoint, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, fmt.Errorf("snapshot checkpoint requires an OpenSearch endpoint")
	}
	if !repositoryName.MatchString(repository) {
		return nil, fmt.Errorf("invalid snapshot repository name %q", repository)
	}
	checkpoint := &SnapshotCheckpoint{
		endpoint:   strings.TrimRight(endpoint, "/"),
		repository: repository,
		client:     &http.Client{Timeout: 60 * time.Second},
	}
	var registered map[string]json.RawMessage
	if err := checkpoint.do(http.MethodGet, "/_snapshot/"+url.PathEscape(repository), nil, &registered); err != nil {
		return nil, fmt.Errorf("snapshot repository %q is not usable: %w; register it as fs with a path.repo the cluster can write", repository, err)
	}
	if _, ok := registered[repository]; !ok {
		return nil, fmt.Errorf("snapshot repository %q is not registered on %s", repository, checkpoint.endpoint)
	}
	return checkpoint, nil
}

// Capture snapshots every index under the namespace's owned prefix and records
// the fingerprint that made those documents a valid starting point.
func (c *SnapshotCheckpoint) Capture(name, fingerprint, namespace string) (Checkpoint, error) {
	if !checkpointName.MatchString(name) {
		return Checkpoint{}, fmt.Errorf("invalid checkpoint name %q", name)
	}
	if fingerprint == "" {
		return Checkpoint{}, fmt.Errorf("checkpoint %s requires a source fingerprint", name)
	}
	prefix := ProjectionPrefix(namespace)
	if prefix == "" {
		return Checkpoint{}, fmt.Errorf("checkpoint %s requires an owned namespace", name)
	}
	documents, shards, err := c.ownedCounts(prefix)
	if err != nil {
		return Checkpoint{}, err
	}
	if len(documents) == 0 {
		return Checkpoint{}, fmt.Errorf("checkpoint %s found no owned indices under %s", name, prefix)
	}
	sources, err := c.capturedSources(documents)
	if err != nil {
		return Checkpoint{}, err
	}
	names := make([]string, 0, len(documents))
	for index := range documents {
		names = append(names, index)
	}
	body := map[string]any{"indices": strings.Join(names, ","), "include_global_state": false}
	var created struct {
		Snapshot struct {
			State  string `json:"state"`
			Shards struct {
				Failed int `json:"failed"`
			} `json:"shards"`
		} `json:"snapshot"`
	}
	path := "/_snapshot/" + url.PathEscape(c.repository) + "/" + url.PathEscape(name) + "?wait_for_completion=true"
	if err := c.do(http.MethodPut, path, body, &created); err != nil {
		return Checkpoint{}, fmt.Errorf("capture checkpoint %s: %w", name, err)
	}
	if created.Snapshot.State != "SUCCESS" || created.Snapshot.Shards.Failed != 0 {
		return Checkpoint{}, fmt.Errorf("checkpoint %s did not complete: state=%q failed=%d",
			name, created.Snapshot.State, created.Snapshot.Shards.Failed)
	}
	return Checkpoint{
		Namespace:   namespace,
		Fingerprint: fingerprint,
		Snapshot:    name,
		Documents:   documents,
		Shards:      shards,
		Sources:     sources,
	}, nil
}

// Restore recreates a checkpoint in the target namespace. The fingerprint must
// still match: a changed scenario turns the captured documents into a stale
// starting point that no read-back check can detect.
func (c *SnapshotCheckpoint) Restore(checkpoint Checkpoint, fingerprint string, sources []SourceRef, namespace string) error {
	if checkpoint.Fingerprint != fingerprint {
		return fmt.Errorf("checkpoint %s was captured for fingerprint %s, not %s: rebuild it",
			checkpoint.Snapshot, checkpoint.Fingerprint, fingerprint)
	}
	// The captured documents name the source repositories and commits the objects
	// were read from, and a search hit hands source_commit back as the candidate
	// basis. A lakeFS commit hashes its creation timestamp and parent list, so
	// rebuilding the same content never reproduces it, and rebuilding a repository
	// leaves the old commit unresolvable (410 once its repository was recreated).
	// Restoring such documents yields candidates that point at unreadable data while
	// every document count still matches, so the identity is compared here.
	if !sameSources(checkpoint.Sources, sources) {
		return fmt.Errorf("checkpoint %s was captured from %s but the target reads %s: rebuild it",
			checkpoint.Snapshot, describeSources(checkpoint.Sources), describeSources(sources))
	}
	source := ProjectionPrefix(checkpoint.Namespace)
	if source == "" {
		return fmt.Errorf("restore of %s requires a captured namespace", checkpoint.Snapshot)
	}
	target := ProjectionPrefix(namespace)
	if target == "" {
		return fmt.Errorf("restore of %s requires an owned target namespace", checkpoint.Snapshot)
	}
	if target == source {
		return fmt.Errorf("restore of %s must target another namespace, not %s", checkpoint.Snapshot, checkpoint.Namespace)
	}
	if err := c.reserve(checkpoint.Shards); err != nil {
		return err
	}
	if err := c.dropOwned(target); err != nil {
		return err
	}
	body := map[string]any{
		"indices":              strings.Join(sortedKeys(checkpoint.Documents), ","),
		"ignore_unavailable":   false,
		"include_global_state": false,
		"rename_pattern":       "^(" + regexp.QuoteMeta(ProjectionPrefix(checkpoint.Namespace)) + ")(.+)$",
		"rename_replacement":   target + "$2",
	}
	var restored struct {
		Snapshot struct {
			Shards struct {
				Failed int `json:"failed"`
			} `json:"shards"`
		} `json:"snapshot"`
	}
	path := "/_snapshot/" + url.PathEscape(c.repository) + "/" + url.PathEscape(checkpoint.Snapshot) + "/_restore?wait_for_completion=true"
	if err := c.do(http.MethodPost, path, body, &restored); err != nil {
		return fmt.Errorf("restore checkpoint %s: %w", checkpoint.Snapshot, err)
	}
	if restored.Snapshot.Shards.Failed != 0 {
		return fmt.Errorf("restore checkpoint %s reported %d failed shards", checkpoint.Snapshot, restored.Snapshot.Shards.Failed)
	}
	return c.verify(checkpoint, target)
}

// Release deletes the captured indices. It is the explicit cleanup path: a
// cancelled run cannot rely on a deferred restore, so releasing by recorded prefix
// keeps the cluster from accumulating checkpoints nobody owns.
func (c *SnapshotCheckpoint) Release(checkpoint Checkpoint) error {
	prefix := ProjectionPrefix(checkpoint.Namespace)
	if prefix == "" {
		return fmt.Errorf("release of %s requires an owned namespace", checkpoint.Snapshot)
	}
	return c.dropOwned(prefix)
}

// verify compares the restored documents with the capture. This is the only
// signal that distinguishes a real restore from a skipped one.
func (c *SnapshotCheckpoint) verify(checkpoint Checkpoint, target string) error {
	refresh := "/" + target + "*/_refresh"
	if err := c.do(http.MethodPost, refresh, nil, nil); err != nil {
		return fmt.Errorf("refresh restored indices: %w", err)
	}
	actual, _, err := c.ownedCounts(target)
	if err != nil {
		return err
	}
	source := ProjectionPrefix(checkpoint.Namespace)
	expected := make(map[string]int64, len(checkpoint.Documents))
	for index, count := range checkpoint.Documents {
		expected[strings.TrimPrefix(index, source)] = count
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("restore of %s left %d indices, capture recorded %d", checkpoint.Snapshot, len(actual), len(expected))
	}
	for suffix, want := range expected {
		got, ok := actual[target+suffix]
		if !ok {
			return fmt.Errorf("restore of %s omitted index %s", checkpoint.Snapshot, target+suffix)
		}
		if got != want {
			return fmt.Errorf("restore of %s left %d documents in %s, capture recorded %d",
				checkpoint.Snapshot, got, target+suffix, want)
		}
	}
	return nil
}

// dropOwned deletes exact index names under the target prefix and then confirms
// the prefix is empty. A wildcard delete and an assumed overwrite are both
// rejected: either could remove or keep documents this invocation does not own.
func (c *SnapshotCheckpoint) dropOwned(target string) error {
	if !ownedPrefix.MatchString(target) {
		return fmt.Errorf("refusing to delete unowned prefix %q", target)
	}
	names, err := c.list(target)
	if err != nil {
		return err
	}
	for _, index := range names {
		if !strings.HasPrefix(index, target) {
			return fmt.Errorf("unowned index in owned inventory: %s", index)
		}
		if err := c.do(http.MethodDelete, "/"+url.PathEscape(index), nil, nil); err != nil {
			return fmt.Errorf("delete owned index %s: %w", index, err)
		}
	}
	remaining, err := c.list(target)
	if err != nil {
		return err
	}
	if len(remaining) > 0 {
		return fmt.Errorf("owned prefix %s still holds %d indices after deletion", target, len(remaining))
	}
	return nil
}

// reserve refuses a restore that would cross the cluster shard limit. The engine
// rejects the whole restore at the limit, so failing first keeps the diagnosis
// here instead of inside a partially attempted restore.
func (c *SnapshotCheckpoint) reserve(required int) error {
	if required <= 0 {
		return fmt.Errorf("restore requires a positive shard reservation")
	}
	limit, err := c.shardLimit()
	if err != nil {
		return err
	}
	used, err := c.openShards()
	if err != nil {
		return err
	}
	if used+required > limit {
		return fmt.Errorf("restore would add %d shards to %d of %d open shards: release checkpoints before restoring", required, used, limit)
	}
	return nil
}

func (c *SnapshotCheckpoint) shardLimit() (int, error) {
	var settings map[string]map[string]any
	if err := c.do(http.MethodGet, "/_cluster/settings?include_defaults=true&flat_settings=true", nil, &settings); err != nil {
		return 0, fmt.Errorf("read cluster shard limit: %w", err)
	}
	for _, scope := range []string{"defaults", "persistent", "transient"} {
		if raw, ok := settings[scope]["cluster.max_shards_per_node"]; ok {
			if value, ok := asInt(raw); ok {
				return value, nil
			}
		}
	}
	return 1000, nil
}

func (c *SnapshotCheckpoint) openShards() (int, error) {
	indices, err := c.settings("/_all/_settings?expand_wildcards=open&flat_settings=true")
	if err != nil {
		return 0, fmt.Errorf("read open indices: %w", err)
	}
	used := 0
	for _, settings := range indices {
		primaries, ok := asInt(settings["index.number_of_shards"])
		if !ok {
			primaries = 1
		}
		replicas, ok := asInt(settings["index.number_of_replicas"])
		if !ok {
			replicas = 1
		}
		used += primaries * (replicas + 1)
	}
	return used, nil
}

// ownedCounts returns live document counts and shard usage for one prefix.
func (c *SnapshotCheckpoint) ownedCounts(prefix string) (map[string]int64, int, error) {
	if !ownedPrefix.MatchString(prefix) {
		return nil, 0, fmt.Errorf("refusing to inspect unowned prefix %q", prefix)
	}
	names, err := c.list(prefix)
	if err != nil {
		return nil, 0, err
	}
	documents := make(map[string]int64, len(names))
	if len(names) == 0 {
		return documents, 0, nil
	}
	for _, name := range names {
		count, err := c.documents(name)
		if err != nil {
			return nil, 0, err
		}
		documents[name] = count
	}
	indices, err := c.settings("/" + strings.Join(names, ",") + "/_settings?expand_wildcards=open&flat_settings=true")
	if err != nil {
		return nil, 0, fmt.Errorf("read owned index settings: %w", err)
	}
	shards := 0
	for _, settings := range indices {
		primaries, ok := asInt(settings["index.number_of_shards"])
		if !ok {
			primaries = 1
		}
		replicas, ok := asInt(settings["index.number_of_replicas"])
		if !ok {
			replicas = 0
		}
		shards += primaries * (replicas + 1)
	}
	return documents, shards, nil
}

func (c *SnapshotCheckpoint) documents(index string) (int64, error) {
	var rows []struct {
		Index string  `json:"index"`
		Count flexInt `json:"docs.count"`
	}
	path := "/_cat/indices/" + url.PathEscape(index) + "?format=json&h=index,docs.count&expand_wildcards=all"
	if err := c.do(http.MethodGet, path, nil, &rows); err != nil {
		return 0, fmt.Errorf("count documents in %s: %w", index, err)
	}
	for _, row := range rows {
		if row.Index == index {
			return int64(row.Count), nil
		}
	}
	return 0, fmt.Errorf("index %s disappeared while counting its documents", index)
}

func (c *SnapshotCheckpoint) list(prefix string) ([]string, error) {
	var rows []struct {
		Index string `json:"index"`
	}
	path := "/_cat/indices/" + prefix + "*?format=json&h=index&expand_wildcards=all"
	if err := c.do(http.MethodGet, path, nil, &rows); err != nil {
		return nil, fmt.Errorf("list owned indices under %s: %w", prefix, err)
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.HasPrefix(row.Index, prefix) {
			names = append(names, row.Index)
		}
	}
	return names, nil
}

func (c *SnapshotCheckpoint) settings(path string) (map[string]map[string]any, error) {
	var indices map[string]struct {
		Settings map[string]any `json:"settings"`
	}
	if err := c.do(http.MethodGet, path, nil, &indices); err != nil {
		return nil, err
	}
	flattened := make(map[string]map[string]any, len(indices))
	for name, index := range indices {
		values := make(map[string]any, len(index.Settings))
		for key, value := range index.Settings {
			values[key] = value
		}
		flattened[name] = values
	}
	return flattened, nil
}

func (c *SnapshotCheckpoint) do(method, path string, body, result any) error {
	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = encoded
	}
	request, err := http.NewRequest(method, c.endpoint+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= 300 {
		return fmt.Errorf("OpenSearch %s %s: HTTP %d: %s", method, path, response.StatusCode, strings.TrimSpace(string(raw)))
	}
	if result == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, result)
}

// flexInt accepts the string form the _cat APIs emit and the numeric form other
// endpoints use, so one decoder serves both.
type flexInt int64

func (f *flexInt) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" || text == "null" {
		*f = 0
		return nil
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("document count %q is not an integer", text)
	}
	*f = flexInt(value)
	return nil
}

func asInt(value any) (int, bool) {
	switch typed := value.(type) {
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	case float64:
		return int(typed), true
	}
	return 0, false
}

func sortedKeys(values map[string]int64) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
