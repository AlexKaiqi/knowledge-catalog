package gitea

import "kc/snapshot/stamp"

// WriteStamp records how to reopen this Gitea member (no token).
func WriteStamp(dir, repositoryID, dsn string) error {
	return stamp.Write(dir, "gitea", repositoryID, dsn)
}

// ReadStamp loads a Gitea pointer directory.
func ReadStamp(dir string) (id, dsn string, err error) {
	return stamp.Read(dir, "gitea")
}
