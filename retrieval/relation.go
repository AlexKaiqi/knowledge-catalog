package retrieval

import (
	"kc/kernel"
	"kc/knowledge"
)

// RelationQuery is the layer ③ one-hop discovery request.
type RelationQuery struct {
	Endpoint     knowledge.KnowledgeRef      `json:"endpoint"`
	RelationType string                      `json:"relationType,omitempty"`
	Role         string                      `json:"role,omitempty"`
	Direction    knowledge.RelationDirection `json:"direction,omitempty"`
}

type RelationRetrieveRequest struct {
	Repository   kernel.RepositoryID `json:"repository"`
	Basis        kernel.CommitID     `json:"basis"`
	Query        RelationQuery       `json:"query"`
	Limit        int                 `json:"limit,omitempty"`
	Continuation string              `json:"continuation,omitempty"`
}

type RelationCandidate struct {
	Repository kernel.RepositoryID `json:"repository"`
	ObjectID   knowledge.ObjectID  `json:"objectId"`
	Basis      kernel.CommitID     `json:"basis"`
	Evidence   []LaneEvidence      `json:"evidence"`
}

type RelationCandidatePage struct {
	Candidates   []RelationCandidate `json:"candidates"`
	Continuation string              `json:"continuation,omitempty"`
	Exhausted    bool                `json:"exhausted"`
}

// RelationRetriever locates candidates only. Complete bodies come from the
// pinned Knowledge authority.
type RelationRetriever interface {
	RetrieveRelations(RelationRetrieveRequest) (RelationCandidatePage, error)
}

type RelationPageRequest struct {
	Query        RelationQuery `json:"query"`
	Limit        int           `json:"limit,omitempty"`
	Continuation string        `json:"continuation,omitempty"`
}

type RelationHit struct {
	KnowledgeRef knowledge.KnowledgeRef      `json:"knowledgeRef"`
	Repository   kernel.RepositoryID         `json:"repository"`
	Commit       kernel.CommitID             `json:"commit"`
	ObjectID     knowledge.ObjectID          `json:"objectId"`
	MatchedRoles []string                    `json:"matchedRoles"`
	Relation     knowledge.CanonicalRelation `json:"relation"`
	Evidence     []LaneEvidence              `json:"evidence,omitempty"`
}

type RelationPage struct {
	RetrievalEvidenceID string        `json:"retrievalEvidenceId,omitempty"`
	SearchView          SearchView    `json:"searchView"`
	Hits                []RelationHit `json:"hits"`
	Claims              []string      `json:"claims,omitempty"`
	Continuation        string        `json:"continuation,omitempty"`
	Exhausted           bool          `json:"exhausted"`
	Generation          string        `json:"generation"`
}

func RelationQueryDigest(query RelationQuery) kernel.Digest {
	return kernel.CanonicalDigest(query)
}
