package retrieval

import (
	"encoding/json"

	"kc/kernel"
	"kc/knowledge"
)

// Traverse access semantics (design: docs/reviewed/retrieval.md):
// a bounded, typed neighborhood closure over stored Relation objects. The
// scope (dataset manifest pin or single-repository pin) is fixed before
// execution; traversal never widens it. Results are a closure, not an
// enumeration of paths: nodes are deduplicated by identity and every selected
// edge keeps its own repository/commit. Unbounded traversal, exhaustive path
// enumeration, shortest-path promises, and any graph query language are
// deliberately out of contract.

// MaxTraverseHops bounds one traversal regardless of caller input.
const MaxTraverseHops = 8

// MaxTraverseEdges bounds the whole continuation sequence, not one page: the
// closure's work is capped even when the caller keeps paging.
const MaxTraverseEdges = 5000

const (
	DefaultTraversePageLimit = 100
	MaxTraversePageLimit     = 1000
)

// TraverseStep is the typed relation pattern every hop must match. Semantics
// mirror RelationQuery: Role matches the endpoint role at the expanding node;
// Direction filters the relation's declared DIRECTED/UNDIRECTED type. Hop
// edges connect every other endpoint of a matched relation; per-hop
// directedness (in/out only) is not part of the first contract.
type TraverseStep struct {
	RelationType string                      `json:"relationType,omitempty"`
	Role         string                      `json:"role,omitempty"`
	Direction    knowledge.RelationDirection `json:"direction,omitempty"`
}

// TraverseQuery fixes the traversal pattern. MinHops filters the returned
// node set by minimum depth; edges stay complete. The scope is supplied by
// the executing channel and is deliberately not part of the query digest.
type TraverseQuery struct {
	Start   knowledge.KnowledgeRef `json:"start"`
	Step    TraverseStep           `json:"step,omitempty"`
	MinHops int                    `json:"minHops,omitempty"`
	MaxHops int                    `json:"maxHops"`
}

func TraverseQueryDigest(query TraverseQuery) kernel.Digest {
	return kernel.CanonicalDigest(query)
}

type TraverseRequest struct {
	Query        TraverseQuery `json:"query"`
	Limit        int           `json:"limit,omitempty"`
	Continuation string        `json:"continuation,omitempty"`
}

// TraverseNode is one reached in-scope object. Nodes are deduplicated by
// (repository, object) across the whole traversal; Depth is the minimum hop
// count from Start. Start itself is never a node.
type TraverseNode struct {
	Repository kernel.RepositoryID `json:"repository"`
	ObjectID   knowledge.ObjectID  `json:"objectId"`
	Depth      int                 `json:"depth"`
}

// TraverseEdge is one selected relation with its own storage identity. Edges
// are deduplicated by (repository, objectId) across the whole traversal; the
// full CanonicalRelation (including out-of-scope endpoint references, which
// are already part of the authorized relation body) is retained.
type TraverseEdge struct {
	Repository kernel.RepositoryID         `json:"repository"`
	Commit     kernel.CommitID             `json:"commit"`
	ObjectID   knowledge.ObjectID          `json:"objectId"`
	Relation   knowledge.CanonicalRelation `json:"relation"`
	Depth      int                         `json:"depth"`
}

// TraverseBoundary marks a frontier endpoint that was not expanded because
// its repository is outside the fixed scope. The reference is already visible
// in the authorized relation body, so the marker discloses nothing new.
type TraverseBoundary struct {
	Repository kernel.RepositoryID `json:"repository"`
	ObjectID   knowledge.ObjectID  `json:"objectId"`
	Reason     string              `json:"reason"`
}

// BoundaryOutsideScope is the only boundary reason in the first contract.
const BoundaryOutsideScope = "outside-scope"

// TraversePage is one delta page of the closure. Pages are disjoint: every
// node and edge is returned exactly once across the continuation sequence,
// and the union of pages is the closure. Exhausted=false always carries a
// continuation plus the claims that explain what remains (pending frontier,
// budget stop, or per-node relation truncation).
type TraversePage struct {
	RetrievalEvidenceID string             `json:"retrievalEvidenceId,omitempty"`
	SearchView          SearchView         `json:"searchView"`
	Nodes               []TraverseNode     `json:"nodes"`
	Edges               []TraverseEdge     `json:"edges"`
	Boundary            []TraverseBoundary `json:"boundary,omitempty"`
	Claims              []string           `json:"claims,omitempty"`
	Continuation        string             `json:"continuation,omitempty"`
	Exhausted           bool               `json:"exhausted"`
}

// TraverseFrontierEntry is one pending expansion node in the saved position.
type TraverseFrontierEntry struct {
	Repository kernel.RepositoryID `json:"repository"`
	Object     knowledge.ObjectID  `json:"object"`
	Depth      int                 `json:"depth"`
}

// EdgeCoordinate identifies one already-selected relation.
type EdgeCoordinate struct {
	Repository kernel.RepositoryID `json:"repository"`
	ObjectID   knowledge.ObjectID  `json:"objectId"`
}

// TraversePosition is the resumable expansion state bound into the
// continuation token. Visited nodes and selected edge identities travel with
// the token so a resumed run reproduces the same closure without
// re-expanding. Positions carries per-node relation lane continuations.
type TraversePosition struct {
	Depth     int                      `json:"depth"`
	Frontier  []TraverseFrontierEntry  `json:"frontier"`
	Visited   []knowledge.KnowledgeRef `json:"visited"`
	Edges     []EdgeCoordinate         `json:"edges"`
	Boundary  []TraverseBoundary       `json:"boundary,omitempty"`
	Claims    []string                 `json:"claims,omitempty"`
	Positions map[string]string        `json:"positions,omitempty"`
}

// TraversePositionKey is the per-node relation lane continuation key used in
// TraversePosition.Positions.
func TraversePositionKey(repository kernel.RepositoryID, object knowledge.ObjectID) string {
	return string(repository) + "\x1f" + string(object)
}

// EncodeTraversePosition binds the expansion state to the traversal query,
// the fixed scope view, and the scope identity (for the dataset channel the
// manifest digest, so a changed file list invalidates the token), then
// encodes it as an opaque continuation token.
func EncodeTraversePosition(query TraverseQuery, view SearchView, identity string, position TraversePosition) (string, error) {
	state := ContinuationState{
		Scope:      "traverse",
		Query:      TraverseQueryDigest(query),
		SearchView: SearchViewDigest(view),
		Projection: kernel.CanonicalDigest(identity),
		Position:   "",
	}
	positionCopy := position
	positionCopy.normalize()
	raw, err := json.Marshal(positionCopy)
	if err != nil {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "traverse position is not serializable: %v", err)
	}
	state.Position = string(raw)
	return EncodeContinuation(state), nil
}

// DecodeTraversePosition verifies the token against the current query, scope
// view, and scope identity. A token from another query, another fixed basis,
// a changed dataset manifest, or a mutated payload is rejected as a
// precondition failure.
func DecodeTraversePosition(token string, query TraverseQuery, view SearchView, identity string) (TraversePosition, error) {
	if token == "" {
		return TraversePosition{}, nil
	}
	state, err := DecodeContinuation(token)
	if err != nil {
		return TraversePosition{}, err
	}
	if state.Scope != "traverse" || state.Query != TraverseQueryDigest(query) ||
		state.SearchView != SearchViewDigest(view) || state.Projection != kernel.CanonicalDigest(identity) {
		return TraversePosition{}, kernel.Fail(kernel.ErrPreconditionFailed,
			"traverse continuation does not match query and fixed scope")
	}
	var position TraversePosition
	if err := kernel.UnmarshalJSON([]byte(state.Position), &position); err != nil {
		return TraversePosition{}, kernel.Fail(kernel.ErrPreconditionFailed, "traverse continuation position is invalid")
	}
	position.normalize()
	return position, nil
}

func (p *TraversePosition) normalize() {
	sortRefs := func(refs []knowledge.KnowledgeRef) {
		for i := 1; i < len(refs); i++ {
			for j := i; j > 0 && refLess(refs[j], refs[j-1]); j-- {
				refs[j], refs[j-1] = refs[j-1], refs[j]
			}
		}
	}
	sortFrontier := func(entries []TraverseFrontierEntry) {
		for i := 1; i < len(entries); i++ {
			for j := i; j > 0 && frontierLess(entries[j], entries[j-1]); j-- {
				entries[j], entries[j-1] = entries[j-1], entries[j]
			}
		}
	}
	sortEdges := func(edges []EdgeCoordinate) {
		for i := 1; i < len(edges); i++ {
			for j := i; j > 0 && edgeLess(edges[j], edges[j-1]); j-- {
				edges[j], edges[j-1] = edges[j-1], edges[j]
			}
		}
	}
	sortFrontier(p.Frontier)
	sortRefs(p.Visited)
	sortEdges(p.Edges)
	sortBoundaries(p.Boundary)
}

func sortBoundaries(boundaries []TraverseBoundary) {
	for i := 1; i < len(boundaries); i++ {
		for j := i; j > 0 && boundaryLess(boundaries[j], boundaries[j-1]); j-- {
			boundaries[j], boundaries[j-1] = boundaries[j-1], boundaries[j]
		}
	}
}

func refLess(left, right knowledge.KnowledgeRef) bool {
	if left.Repository != right.Repository {
		return left.Repository < right.Repository
	}
	return left.Object < right.Object
}

func frontierLess(left, right TraverseFrontierEntry) bool {
	if left.Repository != right.Repository {
		return left.Repository < right.Repository
	}
	if left.Object != right.Object {
		return left.Object < right.Object
	}
	return left.Depth < right.Depth
}

func edgeLess(left, right EdgeCoordinate) bool {
	if left.Repository != right.Repository {
		return left.Repository < right.Repository
	}
	return left.ObjectID < right.ObjectID
}

func boundaryLess(left, right TraverseBoundary) bool {
	if left.Repository != right.Repository {
		return left.Repository < right.Repository
	}
	if left.ObjectID != right.ObjectID {
		return left.ObjectID < right.ObjectID
	}
	return left.Reason < right.Reason
}

// ValidateTraverseRequest enforces the bounded-shape contract: a qualified
// start, explicit hop ceiling within MaxTraverseHops, and a page limit inside
// the relation page bounds.
func ValidateTraverseRequest(request TraverseRequest) (TraverseRequest, error) {
	if request.Query.Start.Repository == "" || request.Query.Start.Object == "" {
		return TraverseRequest{}, kernel.Fail(kernel.ErrUsageInvalid, "traverse requires a qualified start reference")
	}
	if request.Query.MaxHops < 1 || request.Query.MaxHops > MaxTraverseHops {
		return TraverseRequest{}, kernel.Fail(kernel.ErrUsageInvalid, "traverse requires maxHops between 1 and %d", MaxTraverseHops)
	}
	if request.Query.MinHops < 0 || request.Query.MinHops > request.Query.MaxHops {
		return TraverseRequest{}, kernel.Fail(kernel.ErrUsageInvalid, "traverse minHops must be between 0 and maxHops")
	}
	if request.Query.Step.Direction != "" &&
		request.Query.Step.Direction != knowledge.RelationDirected && request.Query.Step.Direction != knowledge.RelationUndirected {
		return TraverseRequest{}, kernel.Fail(kernel.ErrUsageInvalid, "traverse direction must be DIRECTED or UNDIRECTED")
	}
	limit := request.Limit
	if limit < 0 || limit > MaxTraversePageLimit {
		return TraverseRequest{}, kernel.Fail(kernel.ErrUsageInvalid, "traverse limit must be between 1 and %d", MaxTraversePageLimit)
	}
	if limit == 0 {
		limit = DefaultTraversePageLimit
	}
	request.Limit = limit
	return request, nil
}
