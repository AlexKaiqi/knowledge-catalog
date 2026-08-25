package dolt

import (
	"sync"

	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// DoltRepository is a native Dolt Snapshot adapter. Literal repository paths
// are rows in the versioned kc_files table; historical reads use AS OF. It
// never creates a .git directory or delegates authority to FileGit.
type DoltRepository struct {
	repositoryID kernel.RepositoryID
	rootDir      string
	lock         *sync.Mutex
	archived     bool
}

var (
	_             snapshot.Store       = (*DoltRepository)(nil)
	_             snapshot.TreeStore   = (*DoltRepository)(nil)
	_             knowledge.Repository = (*DoltRepository)(nil)
	doltRootLocks sync.Map
)

const (
	doltStamp       = ".kc-dolt-repository"
	doltDockerImage = "dolthub/dolt:latest"
)
