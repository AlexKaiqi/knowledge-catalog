package catalog

import (
	"bytes"
	"strings"

	"kc/kernel"
	"kc/snapshot"
)

// OpenSnapshotRegistry recovers Catalog state from an independent Snapshot
// authority. The store is layer ⓪ tree+CAS, not a Knowledge Repository.
func OpenSnapshotRegistry(store snapshot.Store, catalogID, ref string) (*Registry, error) {
	reg, err := snapshotRegistry(store, catalogID, ref)
	if err != nil {
		return nil, err
	}
	head, err := store.Head(reg.ref)
	if err != nil {
		return nil, err
	}
	if head == "" {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "catalog authority ref does not exist")
	}
	reg.head = string(head)
	state, err := reg.load()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(state.CatalogID) == "" {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "catalog authority ref does not exist")
	}
	if state.CatalogID != catalogID {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "catalog authority identity does not match requested catalog")
	}
	return reg, nil
}

// CreateSnapshotRegistry writes the initial Catalog registry into an already
// provisioned Snapshot authority. It never creates a Knowledge Repository.
func CreateSnapshotRegistry(store snapshot.Store, catalogID, ref string) (*Registry, error) {
	if _, err := OpenSnapshotRegistry(store, catalogID, ref); err == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "catalog authority ref already exists; open it instead")
	} else if kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		return nil, err
	}
	reg, err := snapshotRegistry(store, catalogID, ref)
	if err != nil {
		return nil, err
	}
	head, err := store.Head(reg.ref)
	if err != nil {
		return nil, err
	}
	reg.head = string(head)
	if err := reg.Save(CatalogState{CatalogID: catalogID}, "init "+catalogID, "", "", ""); err != nil {
		return nil, err
	}
	return reg, nil
}

func snapshotRegistry(store snapshot.Store, catalogID, ref string) (*Registry, error) {
	if store == nil || strings.TrimSpace(catalogID) == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "catalog identity and Snapshot authority are required")
	}
	tree, ok := snapshot.TreeStoreOf(store)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "catalog authority requires tree Snapshot capabilities")
	}
	if ref == "" {
		ref = snapshot.DefaultRef
	}
	if !strings.HasPrefix(ref, "refs/heads/") {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "catalog authority requires a branch ref")
	}
	return &Registry{catalogID: catalogID, authority: store, tree: tree, ref: ref}, nil
}

func (g *Registry) snapshotYAML() (map[string][]byte, error) {
	if g.head == "" {
		return map[string][]byte{}, nil
	}
	commit := kernel.CommitID(g.head)
	paths, err := g.tree.ListFiles(commit)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, path := range paths {
		if !strings.HasSuffix(path, ".yaml") || strings.Contains(path, "/") {
			continue
		}
		body, err := g.tree.ReadFile(path, commit)
		if err != nil {
			return nil, err
		}
		out[path] = body
	}
	return out, nil
}

func (g *Registry) saveSnapshot(state CatalogState, message, author, requestID, ruleID string) error {
	current, err := g.load()
	if err != nil {
		return err
	}
	if state.CatalogID != "" && state.CatalogID != g.catalogID {
		return kernel.Fail(kernel.ErrPreconditionFailed, "catalog state identity does not match registry")
	}
	next := NormalizeCatalogState(state)
	next.CatalogID = g.catalogID
	actual, err := g.authority.Head(g.ref)
	if err != nil {
		return err
	}
	if string(actual) != g.head {
		return kernel.Fail(kernel.ErrNonFastForward, "catalog authority moved: expected %s, actual %s", g.head, actual)
	}
	if kernel.CanonicalDigest(current) == kernel.CanonicalDigest(next) {
		return nil
	}
	if message == "" {
		message = "catalog: persist"
	}
	desired, err := yamlFiles(next, g.CatalogID())
	if err != nil {
		return err
	}
	existing, err := g.snapshotYAML()
	if err != nil {
		return err
	}
	changes := treeDiff(existing, desired)
	if len(changes) == 0 {
		return nil
	}
	head := kernel.CommitID(g.head)
	candidate, err := g.tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository:     g.authority.ID(),
		TargetRef:            g.ref,
		BaseCommit:           head,
		ExpectedTargetCommit: head,
		Changes:              changes,
		Message:              message,
		Author:               author,
		RequestID:            requestID,
		RuleID:               ruleID,
	})
	if err != nil {
		return err
	}
	g.head = string(candidate)
	return nil
}

func treeDiff(existing, desired map[string][]byte) []snapshot.TreeChange {
	changes := []snapshot.TreeChange{}
	for path, body := range desired {
		if bytes.Equal(existing[path], body) {
			continue
		}
		changes = append(changes, snapshot.TreeChange{Path: path, Content: body})
	}
	for path := range existing {
		if _, ok := desired[path]; ok {
			continue
		}
		changes = append(changes, snapshot.TreeChange{Path: path, Remove: true})
	}
	return changes
}

func (g *Registry) snapshotHistory(limit int, path string) ([]CatalogCommit, error) {
	if g.head == "" {
		return []CatalogCommit{}, nil
	}
	history, ok := snapshot.HistoryStoreOf(g.authority)
	if !ok {
		return []CatalogCommit{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	ids, err := history.CommitHistory(kernel.CommitID(g.head), limit)
	if err != nil {
		return nil, err
	}
	out := make([]CatalogCommit, 0, len(ids))
	changes, hasChanges := snapshot.ChangeStoreOf(g.authority)
	var previous kernel.CommitID
	for i, id := range ids {
		if path != "" && hasChanges && previous != "" {
			touched, err := changes.ChangedPaths(id, previous)
			if err != nil {
				return nil, err
			}
			hit := false
			for _, changed := range touched {
				if changed == path {
					hit = true
					break
				}
			}
			if !hit {
				previous = id
				continue
			}
		} else if path != "" && i > 0 && !hasChanges {
			current, err := g.tree.ReadFile(path, id)
			if err != nil && kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
				return nil, err
			}
			older, err := g.tree.ReadFile(path, previous)
			if err != nil && kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
				return nil, err
			}
			if bytes.Equal(current, older) {
				previous = id
				continue
			}
		}
		out = append(out, CatalogCommit{Commit: string(id)})
		previous = id
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
