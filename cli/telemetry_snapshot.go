package cli

import (
	"context"
	"fmt"
	"strings"

	"kc/internal/telemetry"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// Snapshot I/O is observed by wrapping the Registry member. Wrappers keep the
// inner method set: a Store-only member stays Store-only, and UnitLocator is
// not invented. Knowledge, Catalog, and Writer keep ordinary type assertions.

type observedSnapshot struct {
	inner   snapshot.Store
	runtime *telemetry.Runtime
	kind    string
}

// Origin forwards the optional authority-instance identity: observation must
// not hide the coordinate that scopes per-repository state such as retrieval
// projections. An inner store without the capability stays without it.
func (s *observedSnapshot) Origin() string {
	if origin, ok := s.inner.(snapshot.StoreOrigin); ok {
		return origin.Origin()
	}
	return ""
}

type observedTreeAuthority struct {
	*observedSnapshot
	tree      snapshot.TreeStore
	directory snapshot.DirectoryReader
	history   snapshot.HistoryStore
	changes   snapshot.ChangeStore
}

type observedKnowledgeTree struct {
	*observedTreeAuthority
	locator knowledge.UnitLocator
}

func observeSnapshotStore(store snapshot.Store, runtime *telemetry.Runtime, fallback string) snapshot.Store {
	if store == nil || runtime == nil {
		return store
	}
	switch store.(type) {
	case *observedSnapshot, *observedTreeAuthority, *observedKnowledgeTree:
		return store
	}
	if _, ok := store.(knowledge.NativeRepository); ok {
		return store
	}
	base := &observedSnapshot{inner: store, runtime: runtime, kind: snapshotStoreKind(store, fallback)}
	tree, hasTree := store.(snapshot.TreeStore)
	directory, hasDirectory := store.(snapshot.DirectoryReader)
	history, hasHistory := store.(snapshot.HistoryStore)
	changes, hasChanges := store.(snapshot.ChangeStore)
	locator, hasLocator := store.(knowledge.UnitLocator)
	if hasTree && hasDirectory && hasHistory && hasChanges {
		authority := &observedTreeAuthority{
			observedSnapshot: base,
			tree:             tree,
			directory:        directory,
			history:          history,
			changes:          changes,
		}
		if hasLocator {
			return &observedKnowledgeTree{observedTreeAuthority: authority, locator: locator}
		}
		return authority
	}
	if !hasTree && !hasDirectory && !hasHistory && !hasChanges && !hasLocator {
		return base
	}
	return store
}

func snapshotStoreKind(store any, fallback string) string {
	type driverNamer interface{ SnapshotDriver() string }
	if namer, ok := store.(driverNamer); ok {
		return boundedTelemetryValue(namer.SnapshotDriver(), "other", "lakefs", "gitea")
	}
	name := fmt.Sprintf("%T", store)
	switch {
	case strings.Contains(name, "SystemRepository"):
		return "other"
	case strings.Contains(name, "lakefs"):
		return "lakefs"
	case strings.Contains(name, "gitea"):
		return "gitea"
	}
	return boundedTelemetryValue(fallback, "other", "lakefs", "gitea")
}

func bindHomeTelemetry(runtime *telemetry.Runtime, ws *Home, flags map[string]FlagValue) {
	if runtime == nil || ws == nil {
		return
	}
	if ws.Store != nil {
		fallback := ""
		if ws.Stores.Repository != "" {
			fallback = ws.Stores.Repository
		}
		ws.Store.SetWrap(func(store snapshot.Store) snapshot.Store {
			return observeSnapshotStore(store, runtime, fallback)
		})
	}
	observeStandingProjection(runtime, ws, flags)
}

func (o *observedSnapshot) ID() kernel.RepositoryID { return o.inner.ID() }

func (o *observedSnapshot) Head(ref string) (kernel.CommitID, error) {
	var commit kernel.CommitID
	err := o.observe("resolve_ref", 0, func() error {
		var innerErr error
		commit, innerErr = o.inner.Head(ref)
		return innerErr
	})
	return commit, err
}

func (o *observedSnapshot) GetRef(ref string) (kernel.CommitID, bool) {
	return o.inner.GetRef(ref)
}

func (o *observedSnapshot) HasCommit(commit kernel.CommitID) bool {
	return o.inner.HasCommit(commit)
}

func (o *observedSnapshot) CreateRef(ref string, commit kernel.CommitID) error {
	return o.observe("commit", 0, func() error { return o.inner.CreateRef(ref, commit) })
}

func (o *observedSnapshot) Merge(targetRef string, candidate, expected kernel.CommitID) (kernel.CommitID, error) {
	var commit kernel.CommitID
	err := o.observe("compare_and_swap", 0, func() error {
		var innerErr error
		commit, innerErr = o.inner.Merge(targetRef, candidate, expected)
		return innerErr
	})
	return commit, err
}

func (o *observedSnapshot) Archived() bool { return o.inner.Archived() }

func (o *observedSnapshot) Archive() error {
	return o.observe("other", 0, o.inner.Archive)
}

func (o *observedSnapshot) Close() error {
	if closer, ok := o.inner.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

func (o *observedTreeAuthority) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	var body []byte
	err := o.observeAfter("read", func() (int64, error) {
		var innerErr error
		body, innerErr = o.tree.ReadFile(path, commit)
		return int64(len(body)), innerErr
	})
	return body, err
}

func (o *observedTreeAuthority) ListFiles(commit kernel.CommitID) ([]string, error) {
	var names []string
	err := o.observe("list_page", 0, func() error {
		var innerErr error
		names, innerErr = o.tree.ListFiles(commit)
		return innerErr
	})
	return names, err
}

func (o *observedTreeAuthority) ApplyTreeCommit(cs snapshot.TreeChangeSet) (kernel.CommitID, error) {
	var commit kernel.CommitID
	err := o.observe("commit", treeChangeBytes(cs), func() error {
		var innerErr error
		commit, innerErr = o.tree.ApplyTreeCommit(cs)
		return innerErr
	})
	return commit, err
}

func (o *observedTreeAuthority) ApplyTreeCommitBulk(cs snapshot.TreeChangeSet) (kernel.CommitID, error) {
	var commit kernel.CommitID
	err := o.observe("commit", treeChangeBytes(cs), func() error {
		bulk, ok := snapshot.BulkTreeIngesterOf(o.inner)
		if !ok {
			return kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"authority %s does not provide bulk ingest", o.inner.ID())
		}
		var innerErr error
		commit, innerErr = bulk.ApplyTreeCommitBulk(cs)
		return innerErr
	})
	return commit, err
}

func (o *observedTreeAuthority) ReadDirectory(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	var page snapshot.DirectoryPage
	err := o.observe("list_page", 0, func() error {
		var innerErr error
		page, innerErr = o.directory.ReadDirectory(request)
		return innerErr
	})
	return page, err
}

func (o *observedTreeAuthority) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	var ids []kernel.CommitID
	err := o.observe("history", 0, func() error {
		var innerErr error
		ids, innerErr = o.history.CommitHistory(commit, limit)
		return innerErr
	})
	return ids, err
}

func (o *observedTreeAuthority) ChangedPaths(from, to kernel.CommitID) ([]string, error) {
	var paths []string
	err := o.observe("diff", 0, func() error {
		var innerErr error
		paths, innerErr = o.changes.ChangedPaths(from, to)
		return innerErr
	})
	return paths, err
}

func (o *observedKnowledgeTree) ObjectUnitPaths(objectID knowledge.ObjectID, commit kernel.CommitID) ([]string, error) {
	var paths []string
	err := o.observe("other", 0, func() error {
		var innerErr error
		paths, innerErr = o.locator.ObjectUnitPaths(objectID, commit)
		return innerErr
	})
	return paths, err
}

func (o *observedSnapshot) observe(operation string, bytes int64, fn func() error) error {
	return o.observeAfter(operation, func() (int64, error) {
		return bytes, fn()
	})
}

func (o *observedSnapshot) observeAfter(operation string, fn func() (int64, error)) error {
	ctx, span, started := o.runtime.StartSnapshot(context.Background(), o.kind, operation)
	bytes, err := fn()
	outcome, errorType := telemetryResult(err)
	if bytes < 0 {
		bytes = 0
	}
	o.runtime.EndSnapshot(ctx, span, started, o.kind, operation, outcome, errorType, bytes)
	return err
}

func treeChangeBytes(cs snapshot.TreeChangeSet) int64 {
	var total int64
	for _, change := range cs.Changes {
		total += int64(len(change.Content))
	}
	return total
}

var _ snapshot.Store = (*observedSnapshot)(nil)
var _ snapshot.Store = (*observedTreeAuthority)(nil)
var _ snapshot.TreeStore = (*observedTreeAuthority)(nil)
var _ snapshot.DirectoryReader = (*observedTreeAuthority)(nil)
var _ snapshot.HistoryStore = (*observedTreeAuthority)(nil)
var _ snapshot.ChangeStore = (*observedTreeAuthority)(nil)
var _ snapshot.TreeStore = (*observedKnowledgeTree)(nil)
var _ knowledge.UnitLocator = (*observedKnowledgeTree)(nil)
