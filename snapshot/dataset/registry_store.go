package dataset

import (
	"bytes"
	"strings"

	"kc/kernel"
	"kc/snapshot"
)

// OpenSnapshotRegistry recovers service state from an independent Snapshot
// authority. The store is layer ⓪ tree+CAS, not a Knowledge Repository.
func OpenSnapshotRegistry(store snapshot.Store, ref string) (Registry, error) {
	reg, err := snapshotRegistry(store, ref)
	if err != nil {
		return nil, err
	}
	head, err := store.Head(reg.ref)
	if err != nil {
		return nil, err
	}
	if head == "" {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "registry authority ref does not exist")
	}
	reg.head = string(head)
	state, err := reg.load()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(state.AuthorityID) == "" {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "registry authority ref does not exist")
	}
	if state.AuthorityID != string(store.ID()) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "registry authority identity does not match Snapshot store")
	}
	return reg, nil
}

// CreateSnapshotRegistry writes the initial service registry into an already
// provisioned Snapshot authority. It never creates a Knowledge Repository.
func CreateSnapshotRegistry(store snapshot.Store, ref string) (Registry, error) {
	if _, err := OpenSnapshotRegistry(store, ref); err == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "registry authority ref already exists; open it instead")
	} else if kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		return nil, err
	}
	reg, err := snapshotRegistry(store, ref)
	if err != nil {
		return nil, err
	}
	head, err := store.Head(reg.ref)
	if err != nil {
		return nil, err
	}
	reg.head = string(head)
	if err := reg.Save(State{AuthorityID: reg.AuthorityID()}, "init "+reg.AuthorityID(), "", "", ""); err != nil {
		return nil, err
	}
	return reg, nil
}

func snapshotRegistry(store snapshot.Store, ref string) (*registry, error) {
	if store == nil || strings.TrimSpace(string(store.ID())) == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "Snapshot registry authority is required")
	}
	tree, ok := snapshot.TreeStoreOf(store)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "registry authority requires tree Snapshot capabilities")
	}
	if ref == "" {
		ref = snapshot.DefaultRef
	}
	if !strings.HasPrefix(ref, "refs/heads/") {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "registry authority requires a branch ref")
	}
	return &registry{authority: store, tree: tree, ref: ref}, nil
}

// An empty current tree is not evidence of an uninitialized authority: an
// accepted registry may have been erased. Only the empty, parentless initial
// Snapshot can precede explicit creation, using the same authority's history.
func (g *registry) requireVirginAuthority() error {
	commit := kernel.CommitID(g.head)
	paths, err := g.tree.ListFiles(commit)
	if err != nil {
		return err
	}
	if len(paths) != 0 {
		return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "registry metadata is missing from a nonempty authority")
	}
	history, ok := snapshot.HistoryStoreOf(g.authority)
	if !ok {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "empty registry authority requires history to prove initial creation")
	}
	commits, err := history.CommitHistory(commit, 2)
	if err != nil {
		return err
	}
	if len(commits) != 1 || commits[0] != commit {
		return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "registry metadata is missing from an existing authority")
	}
	return nil
}

func (g *registry) snapshotYAML() (map[string][]byte, error) {
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

func (g *registry) saveSnapshot(state State, message, author, requestID, ruleID string) error {
	current, err := g.load()
	if err != nil {
		return err
	}
	if state.AuthorityID != "" && state.AuthorityID != g.AuthorityID() {
		return kernel.Fail(kernel.ErrPreconditionFailed, "service state identity does not match registry")
	}
	next := NormalizeState(state)
	next.AuthorityID = g.AuthorityID()
	actual, err := g.authority.Head(g.ref)
	if err != nil {
		return err
	}
	if string(actual) != g.head {
		return kernel.Fail(kernel.ErrNonFastForward, "registry authority moved: expected %s, actual %s", g.head, actual)
	}
	if kernel.CanonicalDigest(current) == kernel.CanonicalDigest(next) {
		return nil
	}
	if message == "" {
		message = "service: persist"
	}
	desired, err := yamlFiles(next, g.AuthorityID())
	if err != nil {
		return err
	}
	existing, err := g.snapshotYAML()
	if err != nil {
		return err
	}
	if err := preserveDefinitionBytes(existing, desired); err != nil {
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
