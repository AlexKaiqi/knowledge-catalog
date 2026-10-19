// Package dataset owns fixed file composition, publication and history.
package dataset

import (
	"kc/kernel"
	"kc/snapshot"
)

// Observer receives the actual operation outcome for external evidence. Its
// error is returned only when the operation succeeded; it cannot replace an
// operation failure or undo an already accepted authority change.
type Observer func(operation string, refs map[string]any, err error) error

// Service is the file Dataset authority. Implementations remain private.
type Service interface {
	AddHook(h Hook)
	CheckResolved(resolved ResolvedDataset) DatasetCheck
	CheckResolvedRepository(resolved ResolvedDataset, repositoryID kernel.RepositoryID) DatasetCheck
	DatasetVersion(setID string, revision int) (Dataset, error)
	DefineDataset(setID string, revision int, sources []DatasetSource) (Dataset, error)
	DumpState() State
	HasRepository(repositoryID kernel.RepositoryID) bool
	LoadState(state State)
	Log(query LogQuery) (History, error)
	NotifySnapshot(ev Snapshot)
	PrepareDataset(setID string, revision int, sources []DatasetSource) (Dataset, error)
	PublishDataset(def Dataset) (Dataset, error)
	ReadView(j Observer) Service
	RequestView(j Observer, as, requestID, ruleID string) Service
	ReadVirtualFile(setID, path string) (VirtualFile, error)
	ReadVirtualFileAt(def Dataset, resolved ResolvedDataset, path string) (VirtualFile, error)
	ReadVirtualFileOf(def Dataset, path string) (VirtualFile, error)
	RecordCreated() error
	RegisterRepository(repositoryID kernel.RepositoryID) error
	ReplayPublished(pin ResolvedDataset) (ResolvedDataset, error)
	Repositories() []string
	Require(repositoryID kernel.RepositoryID) (snapshot.Store, error)
	ResolveDefinition(def Dataset) (ResolvedDataset, error)
	ResolveDefinitionOverlay(def Dataset, overlay map[kernel.RepositoryID]kernel.CommitID) (ResolvedDataset, error)
	ResolveDataset(setID string) (ResolvedDataset, error)
	ResolveDatasetOverlay(setID string, overlay map[kernel.RepositoryID]kernel.CommitID) (ResolvedDataset, error)
	RetireDataset(setID string) error
	Set(setID string) (Dataset, error)
	SetObserver(j Observer)
	SetStamp(as, requestID, ruleID string)
	UnregisterRepository(repositoryID kernel.RepositoryID) error
}

// Registry is the accepted metadata authority opened through Snapshot.
// Its private atomic operations bind Service publication to the same owner.
type Registry interface {
	AuthorityID() string
	Close() error
	Head() (string, error)
	Load() (State, error)
	Save(State, string, string, string, string) error
	CheckAuthority() error
	loadSnapshot() (State, string, error)
	saveExpected(string, State, string, string, string, string) (string, error)
	history(string, int, string) ([]Commit, error)
}

func observe(observer Observer, operation string, refs map[string]any, err error) error {
	if observer != nil {
		recordErr := observer(operation, refs, err)
		if err == nil {
			return recordErr
		}
	}
	return err
}

// DatasetDeliveredTree indexes the per-file entries of a published item list.
// Prefix entries are composed separately by the file gateway.
type DatasetDeliveredTree interface {
	Find(target string) (DatasetItem, bool)
	FilesIn(dir string) []DatasetItem
	DirsIn(dir string) []string
}
