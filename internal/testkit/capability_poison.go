package testkit

import (
	"sync"

	"kc/kernel"
	"kc/snapshot"
)

// CapabilityPoisonStore deliberately implements only snapshot.Store. It is a
// conformance fixture for proving that missing upper capabilities fail closed
// instead of returning empty data, scanning, or panicking.
type CapabilityPoisonStore struct {
	mu       sync.RWMutex
	id       kernel.RepositoryID
	root     kernel.CommitID
	archived bool
}

func NewCapabilityPoisonStore(id kernel.RepositoryID) *CapabilityPoisonStore {
	return &CapabilityPoisonStore{id: id, root: "poison-root"}
}

func (s *CapabilityPoisonStore) ID() kernel.RepositoryID { return s.id }
func (s *CapabilityPoisonStore) Head(string) (kernel.CommitID, error) {
	return s.root, nil
}
func (s *CapabilityPoisonStore) GetRef(string) (kernel.CommitID, bool) {
	return s.root, true
}
func (s *CapabilityPoisonStore) HasCommit(commit kernel.CommitID) bool { return commit == s.root }
func (s *CapabilityPoisonStore) CreateRef(string, kernel.CommitID) error {
	return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "poison store has no ref mutation")
}
func (s *CapabilityPoisonStore) Merge(string, kernel.CommitID, kernel.CommitID) (kernel.CommitID, error) {
	return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied, "poison store has no merge")
}
func (s *CapabilityPoisonStore) Archived() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.archived
}
func (s *CapabilityPoisonStore) Archive() error {
	s.mu.Lock()
	s.archived = true
	s.mu.Unlock()
	return nil
}

var _ snapshot.Store = (*CapabilityPoisonStore)(nil)
