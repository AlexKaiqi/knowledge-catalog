package home

import (
	"kc/kernel"
	"kc/snapshot"
)

// restoreManagedSource preserves the driver's exact optional capabilities.
// Only drivers that explicitly provide a deferred handle avoid an online open;
// native Dolt sources continue to return their original native implementation.
func restoreManagedSource(record managedRecord) (snapshot.Store, error) {
	driver, err := authorityFor(record.Binding.Driver)
	if err != nil {
		return nil, err
	}
	if driver.managedRestore != nil {
		return driver.managedRestore(record), nil
	}
	return openVerifiedManagedSource(record)
}

func openVerifiedManagedSource(record managedRecord) (snapshot.Store, error) {
	driver, err := authorityFor(record.Binding.Driver)
	if err != nil {
		return nil, err
	}
	source, err := driver.managedOpen(record.Binding, record.AllocationID, record.BackendID)
	if err != nil {
		return nil, err
	}
	if !source.HasCommit(record.Head) {
		closeManagedSource(source)
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "managed initial published commit is unavailable")
	}
	return source, nil
}

// managedTreeSource is selected only for Gitea by the composition root. It
// carries durable coordinates, never cached authority data. Every operation
// verifies the original allocation, numeric backend identity and initial commit
// before using the real adapter. The four capabilities match Gitea exactly.
type managedTreeSource struct{ record managedRecord }

func managedTreeCall[T any](s *managedTreeSource, fn func(snapshot.Store) (T, error)) (T, error) {
	var zero T
	source, err := openVerifiedManagedSource(s.record)
	if err != nil {
		return zero, err
	}
	defer closeManagedSource(source)
	return fn(source)
}
func managedTreeDo(s *managedTreeSource, fn func(snapshot.Store) error) error {
	_, err := managedTreeCall(s, func(source snapshot.Store) (struct{}, error) { return struct{}{}, fn(source) })
	return err
}
func (s *managedTreeSource) ID() kernel.RepositoryID { return kernel.RepositoryID(s.record.Binding.ID) }
func (s *managedTreeSource) Head(ref string) (kernel.CommitID, error) {
	return managedTreeCall(s, func(source snapshot.Store) (kernel.CommitID, error) { return source.Head(ref) })
}
func (s *managedTreeSource) GetRef(ref string) (kernel.CommitID, bool) {
	type result struct {
		commit kernel.CommitID
		ok     bool
	}
	value, err := managedTreeCall(s, func(source snapshot.Store) (result, error) {
		commit, ok := source.GetRef(ref)
		return result{commit, ok}, nil
	})
	return value.commit, err == nil && value.ok
}
func (s *managedTreeSource) HasCommit(commit kernel.CommitID) bool {
	value, err := managedTreeCall(s, func(source snapshot.Store) (bool, error) { return source.HasCommit(commit), nil })
	return err == nil && value
}
func (s *managedTreeSource) CreateRef(ref string, commit kernel.CommitID) error {
	return managedTreeDo(s, func(source snapshot.Store) error { return source.CreateRef(ref, commit) })
}
func (s *managedTreeSource) Merge(ref string, candidate, expected kernel.CommitID) (kernel.CommitID, error) {
	return managedTreeCall(s, func(source snapshot.Store) (kernel.CommitID, error) { return source.Merge(ref, candidate, expected) })
}
func (s *managedTreeSource) Archived() bool {
	value, err := managedTreeCall(s, func(source snapshot.Store) (bool, error) { return source.Archived(), nil })
	return err != nil || value
}
func (s *managedTreeSource) Archive() error {
	return managedTreeDo(s, func(source snapshot.Store) error { return source.Archive() })
}
func (s *managedTreeSource) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	return managedTreeCall(s, func(source snapshot.Store) ([]byte, error) { return source.(snapshot.TreeStore).ReadFile(path, commit) })
}
func (s *managedTreeSource) ListFiles(commit kernel.CommitID) ([]string, error) {
	return managedTreeCall(s, func(source snapshot.Store) ([]string, error) { return source.(snapshot.TreeStore).ListFiles(commit) })
}
func (s *managedTreeSource) ApplyTreeCommit(change snapshot.TreeChangeSet) (kernel.CommitID, error) {
	return managedTreeCall(s, func(source snapshot.Store) (kernel.CommitID, error) {
		return source.(snapshot.TreeStore).ApplyTreeCommit(change)
	})
}
func (s *managedTreeSource) ReadDirectory(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	return managedTreeCall(s, func(source snapshot.Store) (snapshot.DirectoryPage, error) {
		return source.(snapshot.DirectoryReader).ReadDirectory(request)
	})
}
func (s *managedTreeSource) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	return managedTreeCall(s, func(source snapshot.Store) ([]kernel.CommitID, error) {
		return source.(snapshot.HistoryStore).CommitHistory(commit, limit)
	})
}

var _ snapshot.Store = (*managedTreeSource)(nil)
var _ snapshot.TreeStore = (*managedTreeSource)(nil)
var _ snapshot.DirectoryReader = (*managedTreeSource)(nil)
var _ snapshot.HistoryStore = (*managedTreeSource)(nil)
