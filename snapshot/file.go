package snapshot

import "kc/kernel"

// FileRef locates an opaque file in one immutable Snapshot.
type FileRef struct {
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Path       string              `json:"path"`
	Digest     kernel.Digest       `json:"digest,omitempty"`
}
