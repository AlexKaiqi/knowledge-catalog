package lakefs

import "kc/snapshot/stamp"

// WriteStamp records only the public location needed to reopen a member.
// Credentials remain in Server-private storage or the deployment environment.
func WriteStamp(dir, repositoryID, dsn string) error {
	return stamp.Write(dir, "lakefs", repositoryID, dsn)
}

// ReadStamp loads a lakeFS pointer directory.
func ReadStamp(dir string) (id, dsn string, err error) {
	return stamp.Read(dir, "lakefs")
}
