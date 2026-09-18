package dolt

import (
	"sync"

	"kc/kernel"
	"kc/snapshot"
)

// DoltRepository is a native Dolt Snapshot adapter. Literal repository paths
// are rows in the versioned kc_files table; historical reads use AS OF. It
// never creates a .git directory or delegates authority to another adapter.
type DoltRepository struct {
	managedAllocation string
	repositoryID      kernel.RepositoryID
	rootDir           string
	lock              *sync.Mutex
	archived          bool
}

// Close releases this database's live query session, and with it the write
// lease. snapshot.Registry.Close calls it at the end of the command or request
// that opened the Repository; a later read starts a new session.
func (r *DoltRepository) Close() error {
	closeEngineAt(r.rootDir)
	return nil
}

var (
	_             snapshot.Store           = (*DoltRepository)(nil)
	_             snapshot.TreeStore       = (*DoltRepository)(nil)
	_             snapshot.DirectoryReader = (*DoltRepository)(nil)
	_             snapshot.HistoryStore    = (*DoltRepository)(nil)
	doltRootLocks sync.Map
)

const (
	doltStamp       = ".kc-dolt-repository"
	doltDockerImage = "dolthub/dolt:latest"
)
