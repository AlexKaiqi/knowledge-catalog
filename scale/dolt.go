package scale

import (
	"kc/kernel"
	"kc/local"
	"kc/repository"
)

// DoltRepository is the scale Snapshot adapter (layer ⓪): same SnapshotStore
// + Knowledge口 as FileGit. Storage is still git-shaped knowledge files
// (commit / ref / CAS) until native Dolt SQL mapping is assembled.
// APPEND is a separate Stream; do not repo-add a stream.
type DoltRepository struct {
	*local.FileGitRepository
}

var (
	_ repository.Repository    = (*DoltRepository)(nil)
	_ repository.SnapshotStore = (*DoltRepository)(nil)
	_ repository.Knowledge     = (*DoltRepository)(nil)
)

// OpenDolt opens a git-shaped Snapshot stamped driver=dolt.
func OpenDolt(rootDir string, id kernel.RepositoryID) (repository.Repository, error) {
	inner, err := local.OpenGitSnapshot(rootDir, id, "dolt")
	if err != nil {
		return nil, err
	}
	return &DoltRepository{FileGitRepository: inner}, nil
}
