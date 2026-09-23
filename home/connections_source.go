package home

import (
	"kc/kernel"
	"kc/snapshot"
)

// connectedSource restores a fixed Gitea authority without contacting it.
// Each operation reads the current private receipt, so credential rotation
// reaches existing readers without replacing the authority in the registry.
// Its optional capabilities match the supported Gitea adapter exactly.
type connectedSource struct {
	dir string
	id  kernel.RepositoryID
}

func connectionCall[T any](s *connectedSource, fn func(snapshot.Store) (T, error)) (T, error) {
	var zero T
	r, credential, err := readConnection(s.dir, string(s.id))
	if err != nil {
		return zero, err
	}
	source, err := openConnectionRecord(r, credential)
	if err != nil {
		return zero, err
	}
	defer closeManagedSource(source)
	result, err := fn(source)
	return result, connectionFailure(err)
}
func connectionDo(s *connectedSource, fn func(snapshot.Store) error) error {
	_, err := connectionCall(s, func(source snapshot.Store) (struct{}, error) { return struct{}{}, fn(source) })
	return err
}
func (s *connectedSource) ID() kernel.RepositoryID { return s.id }

// Origin forwards the optional authority-instance identity: the connected
// authority's binding DSN is the stable, non-secret coordinate that keeps
// per-repository state (retrieval projections) scoped to one authority
// instance. A missing connection yields an empty origin and falls back to the
// logical repository id.
func (s *connectedSource) Origin() string {
	record, _, err := readConnection(s.dir, string(s.id))
	if err != nil {
		return ""
	}
	return record.Binding.DSN
}
func (s *connectedSource) Head(ref string) (kernel.CommitID, error) {
	return connectionCall(s, func(source snapshot.Store) (kernel.CommitID, error) { return source.Head(ref) })
}
func (s *connectedSource) GetRef(ref string) (kernel.CommitID, bool) {
	type value struct {
		commit kernel.CommitID
		ok     bool
	}
	v, err := connectionCall(s, func(source snapshot.Store) (value, error) {
		commit, ok := source.GetRef(ref)
		return value{commit, ok}, nil
	})
	return v.commit, err == nil && v.ok
}
func (s *connectedSource) HasCommit(commit kernel.CommitID) bool {
	v, err := connectionCall(s, func(source snapshot.Store) (bool, error) { return source.HasCommit(commit), nil })
	return err == nil && v
}
func (s *connectedSource) CreateRef(ref string, commit kernel.CommitID) error {
	return connectionDo(s, func(source snapshot.Store) error { return source.CreateRef(ref, commit) })
}
func (s *connectedSource) Merge(ref string, candidate, expected kernel.CommitID) (kernel.CommitID, error) {
	return connectionCall(s, func(source snapshot.Store) (kernel.CommitID, error) { return source.Merge(ref, candidate, expected) })
}
func (s *connectedSource) Archived() bool {
	v, err := connectionCall(s, func(source snapshot.Store) (bool, error) { return source.Archived(), nil })
	return err != nil || v
}
func (s *connectedSource) Archive() error {
	return connectionDo(s, func(source snapshot.Store) error { return source.Archive() })
}
func (s *connectedSource) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	return connectionCall(s, func(source snapshot.Store) ([]byte, error) {
		tree, err := requireTreeCapability(source)
		if err != nil {
			return nil, err
		}
		return tree.ReadFile(path, commit)
	})
}
func (s *connectedSource) ListFiles(commit kernel.CommitID) ([]string, error) {
	return connectionCall(s, func(source snapshot.Store) ([]string, error) {
		tree, err := requireTreeCapability(source)
		if err != nil {
			return nil, err
		}
		return tree.ListFiles(commit)
	})
}
func (s *connectedSource) ApplyTreeCommit(changes snapshot.TreeChangeSet) (kernel.CommitID, error) {
	return connectionCall(s, func(source snapshot.Store) (kernel.CommitID, error) {
		tree, err := requireTreeCapability(source)
		if err != nil {
			return "", err
		}
		return tree.ApplyTreeCommit(changes)
	})
}
func (s *connectedSource) ReadDirectory(q snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	return connectionCall(s, func(source snapshot.Store) (snapshot.DirectoryPage, error) {
		directory, err := requireDirectoryCapability(source)
		if err != nil {
			return snapshot.DirectoryPage{}, err
		}
		return directory.ReadDirectory(q)
	})
}
func (s *connectedSource) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	return connectionCall(s, func(source snapshot.Store) ([]kernel.CommitID, error) {
		history, err := requireHistoryCapability(source)
		if err != nil {
			return nil, err
		}
		return history.CommitHistory(commit, limit)
	})
}

func (s *connectedSource) ChangedPaths(from, to kernel.CommitID) ([]string, error) {
	return connectionCall(s, func(source snapshot.Store) ([]string, error) {
		changes, err := requireChangeCapability(source)
		if err != nil {
			return nil, err
		}
		return changes.ChangedPaths(from, to)
	})
}

var _ snapshot.Store = (*connectedSource)(nil)
var _ snapshot.TreeStore = (*connectedSource)(nil)
var _ snapshot.DirectoryReader = (*connectedSource)(nil)
var _ snapshot.HistoryStore = (*connectedSource)(nil)
