package knowledge

import "kc/kernel"

type ObjectRevision struct {
	Commit            kernel.CommitID  `json:"commit"`
	Status            ResolutionStatus `json:"status"`
	Digest            kernel.Digest    `json:"digest,omitempty"`
	DeclarationDigest kernel.Digest    `json:"declarationDigest,omitempty"`
}

type ObjectDiff struct {
	ObjectID   ObjectID        `json:"objectId"`
	FromCommit kernel.CommitID `json:"fromCommit"`
	ToCommit   kernel.CommitID `json:"toCommit"`
	From       *KnowledgeValue `json:"from,omitempty"`
	To         *KnowledgeValue `json:"to,omitempty"`
}
