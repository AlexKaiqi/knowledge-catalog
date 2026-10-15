package discovery

import (
	"context"
	"kc/kernel"
	objectmodel "kc/object"
)

// RelationQuery is the layer ③ one-hop discovery request.
type RelationQuery struct {
	Endpoint     objectmodel.ObjectID          `json:"endpoint"`
	RelationType string                        `json:"relationType,omitempty"`
	Role         string                        `json:"role,omitempty"`
	Direction    objectmodel.RelationDirection `json:"direction,omitempty"`
}

type RelationRetrieveRequest struct {
	// Repository/Basis locate the stored relations. The endpoint in Query is
	// an object ID only, so the stored relation and its endpoints never have
	// to share a Repository.
	Repository   kernel.RepositoryID `json:"repository"`
	Basis        kernel.CommitID     `json:"basis"`
	Query        RelationQuery       `json:"query"`
	Limit        int                 `json:"limit,omitempty"`
	Continuation string              `json:"continuation,omitempty"`
}

type RelationCandidate struct {
	Repository kernel.RepositoryID  `json:"repository"`
	ObjectID   objectmodel.ObjectID `json:"objectId"`
	Basis      kernel.CommitID      `json:"basis"`
	Evidence   []LaneEvidence       `json:"evidence"`
}

type RelationCandidatePage struct {
	Candidates   []RelationCandidate `json:"candidates"`
	Continuation string              `json:"continuation,omitempty"`
	Exhausted    bool                `json:"exhausted"`
}

// RelationRetriever locates candidates only. Complete bodies come from the
// pinned Object authority.
type RelationRetriever interface {
	RetrieveRelations(RelationRetrieveRequest) (RelationCandidatePage, error)
}

// ContextRelationRetriever is the optional cancellable relation provider port.
type ContextRelationRetriever interface {
	RetrieveRelationsContext(context.Context, RelationRetrieveRequest) (RelationCandidatePage, error)
}

type RelationPageRequest struct {
	Query        RelationQuery `json:"query"`
	Limit        int           `json:"limit,omitempty"`
	Continuation string        `json:"continuation,omitempty"`
}

type RelationHit struct {
	ObjectRef    objectmodel.ObjectRef         `json:"objectRef"`
	Repository   kernel.RepositoryID           `json:"repository"`
	Commit       kernel.CommitID               `json:"commit"`
	ObjectID     objectmodel.ObjectID          `json:"objectId"`
	MatchedRoles []string                      `json:"matchedRoles"`
	Relation     objectmodel.CanonicalRelation `json:"relation"`
	Evidence     []LaneEvidence                `json:"evidence,omitempty"`
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
