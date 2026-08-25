package knowledge

import "kc/kernel"

// ObjectID is stable within one knowledge repository and independent of its
// on-disk path. It belongs to layer ②; Snapshot and Catalog coordinates do not
// carry it.
type ObjectID string

// KnowledgeRef is a long-lived, path-independent reference to knowledge.
type KnowledgeRef struct {
	Repository kernel.RepositoryID `json:"repository"`
	Object     ObjectID            `json:"object"`
}

// PinnedKnowledgeRef fixes a KnowledgeRef to one immutable Snapshot commit.
type PinnedKnowledgeRef struct {
	KnowledgeRef
	Commit kernel.CommitID `json:"commit"`
}

func FormatKnowledgeRef(repository kernel.RepositoryID, object ObjectID) string {
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
