// Package dataset owns fixed file composition, publication and history.
package dataset

import (
	"kc/kernel"
	"kc/snapshot"
)

// Observer receives the actual operation outcome for external evidence.
type Observer func(operation string, refs map[string]any, err error) error

// Service is the file Dataset authority. Implementations remain private.
type Service interface {
	AddHook(h Hook)
	CheckResolved(resolved ResolvedKnowledgeSet) KnowledgeSetCheck
	CheckResolvedRepository(resolved ResolvedKnowledgeSet, repositoryID kernel.RepositoryID) KnowledgeSetCheck
	DatasetVersion(setID string, revision int) (KnowledgeSet, error)
	DefineKnowledgeSet(setID string, revision int, sources []KnowledgeSetSource) (KnowledgeSet, error)
	DumpState() State
	HasRepository(repositoryID kernel.RepositoryID) bool
	LoadState(state State)
	Log(query LogQuery) History
	NotifySnapshot(ev Snapshot)
	PrepareKnowledgeSet(setID string, revision int, sources []KnowledgeSetSource) (KnowledgeSet, error)
	PublishKnowledgeSet(def KnowledgeSet) (KnowledgeSet, error)
	ReadView(j Observer) Service
	ReadVirtualFile(setID, path string) (VirtualFile, error)
	ReadVirtualFileAt(def KnowledgeSet, resolved ResolvedKnowledgeSet, path string) (VirtualFile, error)
	ReadVirtualFileOf(def KnowledgeSet, path string) (VirtualFile, error)
	RecordCreated() error
	RegisterRepository(repositoryID kernel.RepositoryID) error
	ReplayPublished(pin ResolvedKnowledgeSet) (ResolvedKnowledgeSet, error)
	Repositories() []string
	Require(repositoryID kernel.RepositoryID) (snapshot.Store, error)
	ResolveDefinition(def KnowledgeSet) (ResolvedKnowledgeSet, error)
	ResolveDefinitionOverlay(def KnowledgeSet, overlay map[kernel.RepositoryID]kernel.CommitID) (ResolvedKnowledgeSet, error)
	ResolveKnowledgeSet(setID string) (ResolvedKnowledgeSet, error)
	ResolveKnowledgeSetOverlay(setID string, overlay map[kernel.RepositoryID]kernel.CommitID) (ResolvedKnowledgeSet, error)
	RetireKnowledgeSet(setID string) error
	Set(setID string) (KnowledgeSet, error)
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
	history(int, string) ([]Commit, error)
}

func observe(observer Observer, operation string, refs map[string]any, err error) error {
	if observer != nil {
		return observer(operation, refs, err)
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
