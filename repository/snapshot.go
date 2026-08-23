package repository

import "kc/kernel"

// Snapshot is a repo Ref moving from → to (COMMIT / merge event). Not the
// SnapshotStore interface and not an index type.
// ObjectIDs is the changed set; nil means unknown; empty means no object edits.
// Repository is the ⓪ member: a plain git repo also advances, subscribers that
// need layer ② check the capability themselves.
type Snapshot struct {
	Repository SnapshotStore
	From       kernel.CommitID
	To         kernel.CommitID
	ObjectIDs  []kernel.ObjectID
}

// OnSnapshot registers a listener. Writer COMMIT and ControlPlane merge emit;
// Catalog subscribes at construct. Failure in a listener must not roll back the write.
func (s *Store) OnSnapshot(fn func(Snapshot)) {
	if s == nil || fn == nil {
		return
	}
	s.onSnapshot = append(s.onSnapshot, fn)
}

func (s *Store) NotifySnapshot(ev Snapshot) {
	if s == nil || ev.Repository == nil {
		return
	}
	for _, fn := range s.onSnapshot {
		if fn != nil {
			fn(ev)
		}
	}
}
