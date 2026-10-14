package object

import "kc/kernel"

// ObjectID is stable within one object repository and independent of its
// on-disk path. It belongs to layer ②; Snapshot and Dataset service coordinates do not
// carry it.
type ObjectID string

// ObjectRef is a long-lived, path-independent reference to objectmodel.
type ObjectRef struct {
	Repository kernel.RepositoryID `json:"repository"`
	Object     ObjectID            `json:"object"`
}

// PinnedObjectRef fixes an ObjectRef to one immutable Snapshot commit.
type PinnedObjectRef struct {
	ObjectRef
	Commit kernel.CommitID `json:"commit"`
}

func FormatObjectRef(repository kernel.RepositoryID, object ObjectID) string {
	return "kc://" + shortRepository(repository) + "/" + string(object)
}

func FormatPinnedRef(repository kernel.RepositoryID, commit kernel.CommitID, object ObjectID) string {
	return "kc://" + shortRepository(repository) + "@" + string(commit) + "/" + string(object)
}

func shortRepository(id kernel.RepositoryID) string {
	s := string(id)
	if len(s) >= 5 && s[:5] == "kr://" {
		return s[5:]
	}
	return s
}
