package knowledge

import "kc/kernel"

const DefaultObjectLogLimit = 50

type ObjectRevision struct {
	Commit            kernel.CommitID  `json:"commit"`
	Status            ResolutionStatus `json:"status"`
	Digest            kernel.Digest    `json:"digest,omitempty"`
	DeclarationDigest kernel.Digest    `json:"declarationDigest,omitempty"`
}

// ObjectLogQuery pages the introducing commits of one object. Limit 0 means
// DefaultObjectLogLimit. After is exclusive: the previous page's last commit.
type ObjectLogQuery struct {
	Limit int             `json:"limit,omitempty"`
	After kernel.CommitID `json:"after,omitempty"`
}

func (q ObjectLogQuery) PageSize() int {
	if q.Limit <= 0 {
		return DefaultObjectLogLimit
	}
	return q.Limit
}

type ObjectDiff struct {
	ObjectID   ObjectID        `json:"objectId"`
	FromCommit kernel.CommitID `json:"fromCommit"`
	ToCommit   kernel.CommitID `json:"toCommit"`
	From       *KnowledgeValue `json:"from,omitempty"`
	To         *KnowledgeValue `json:"to,omitempty"`
}
