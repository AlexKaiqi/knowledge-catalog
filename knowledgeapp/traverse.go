package knowledgeapp

import (
	"context"
	"sort"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
)

// Traversal executes the bounded neighborhood closure contract from
// docs/KNOWLEDGE_CATALOG_DESIGN.md §7.5: the scope is fixed before execution,
// every hop reuses the one-hop relation execution lane at the pinned basis,
// authorization is checked before each expansion, and pages are disjoint
// deltas whose union is the closure. Edges only come from the expanding
// node's own storage repository; frontier endpoints outside the fixed scope
// stop the traversal and are reported as explicit boundaries.

// TraverseScope is the fixed expansion range resolved by the calling channel.
// Identity is the canonical scope identity bound into continuation tokens:
// the dataset manifest digest for the dataset channel, the pinned commit for
// the single-repository channel.
type TraverseScope struct {
	Snapshots map[kernel.RepositoryID]kernel.CommitID
	Contains  func(kernel.RepositoryID, knowledge.ObjectID) (bool, error)
	Identity  string
}

// TraverseProjection is the per-hop relation execution port: the same lane
// one-hop RELATIONS uses, never an authority scan.
type TraverseProjection interface {
	RelationsAtContext(context.Context, knowledge.Repository, kernel.CommitID, retrieval.RelationPageRequest) (retrieval.RelationPage, error)
}

// TraverseCore runs one traversal page against a fixed scope.
type TraverseCore struct {
	Repositories RepositoryLookup
	Projection   TraverseProjection
}

// Run expands the closure until the page limit, the edge budget, or the
// frontier is exhausted, and returns the delta plus a resumable position.
func (c TraverseCore) Run(ctx context.Context, request retrieval.TraverseRequest, scope TraverseScope) (retrieval.TraversePage, error) {
	if c.Projection == nil || c.Repositories == nil {
		return retrieval.TraversePage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "traverse services are incomplete")
	}
	if scope.Contains == nil || len(scope.Snapshots) == 0 {
		return retrieval.TraversePage{}, kernel.Fail(kernel.ErrUsageInvalid, "traverse requires a fixed scope")
	}
	view := retrieval.SearchView{Snapshots: scope.Snapshots}
	position, err := retrieval.DecodeTraversePosition(request.Continuation, request.Query, view, scope.Identity)
	if err != nil {
		return retrieval.TraversePage{}, err
	}
	visited := make(map[knowledge.KnowledgeRef]struct{}, len(position.Visited))
	for _, ref := range position.Visited {
		visited[ref] = struct{}{}
	}
	edgesSeen := make(map[retrieval.EdgeCoordinate]struct{}, len(position.Edges))
	for _, edge := range position.Edges {
		edgesSeen[edge] = struct{}{}
	}
	frontier := append([]retrieval.TraverseFrontierEntry(nil), position.Frontier...)
	positions := make(map[string]string, len(position.Positions))
	for key, value := range position.Positions {
		positions[key] = value
	}
	boundarySeen := make(map[retrieval.TraverseBoundary]struct{}, len(position.Boundary))
	for _, item := range position.Boundary {
		boundarySeen[item] = struct{}{}
	}
	claimsSeen := make(map[string]struct{}, len(position.Claims))
	for _, claim := range position.Claims {
		claimsSeen[claim] = struct{}{}
	}

	if len(frontier) == 0 && len(visited) == 0 {
		start := request.Query.Start
		allowed, err := scope.Contains(start.Repository, start.Object)
		if err != nil {
			return retrieval.TraversePage{}, err
		}
		if !allowed {
			return retrieval.TraversePage{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "traverse start is outside the fixed scope")
		}
		visited[start] = struct{}{}
		frontier = []retrieval.TraverseFrontierEntry{{Repository: start.Repository, Object: start.Object, Depth: 0}}
	}

	out := retrieval.TraversePage{SearchView: view, Nodes: []retrieval.TraverseNode{}, Edges: []retrieval.TraverseEdge{}}
	appendClaim := func(claim string) {
		if _, seen := claimsSeen[claim]; seen {
			return
		}
		claimsSeen[claim] = struct{}{}
		position.Claims = append(position.Claims, claim)
		out.Claims = append(out.Claims, claim)
	}
	budgetStop := func(reason string) {
		appendClaim("traversal stopped: " + reason)
		out.Exhausted = false
	}
	pageDone := func() error {
		position.Frontier = frontier
		position.Visited = refsOf(visited)
		position.Edges = edgeRefsOf(edgesSeen)
		position.Boundary = boundaryKeys(boundarySeen)
		position.Positions = positions
		out.Boundary = sortBoundaries(out.Boundary)
		out.Continuation, err = retrieval.EncodeTraversePosition(request.Query, view, scope.Identity, position)
		return err
	}

	for len(frontier) > 0 {
		if len(out.Nodes) >= request.Limit {
			if err := pageDone(); err != nil {
				return retrieval.TraversePage{}, err
			}
			return out, nil
		}
		if len(edgesSeen) >= retrieval.MaxTraverseEdges {
			budgetStop("traverse edge budget exhausted")
			if err := pageDone(); err != nil {
				return retrieval.TraversePage{}, err
			}
			return out, nil
		}
		entry := frontier[0]
		if entry.Depth >= request.Query.MaxHops {
			// Breadth-first order means every remaining entry is at or past
			// the hop ceiling: nothing further is part of this closure.
			frontier = frontier[:0]
			break
		}
		frontier = frontier[1:]
		commit, inScope := scope.Snapshots[entry.Repository]
		if !inScope {
			return retrieval.TraversePage{}, kernel.Fail(kernel.ErrPreconditionFailed,
				"traverse frontier repository %s is outside the fixed scope", entry.Repository)
		}
		repo, err := c.Repositories.Require(entry.Repository, kernel.ErrCapabilityUnsatisfied)
		if err != nil {
			return retrieval.TraversePage{}, err
		}
		nodeRef := knowledge.KnowledgeRef{Repository: entry.Repository, Object: entry.Object}
		key := retrieval.TraversePositionKey(entry.Repository, entry.Object)
		laneContinuation := positions[key]
		pageRequest := retrieval.RelationPageRequest{
			Query: retrieval.RelationQuery{
				Endpoint:     nodeRef,
				RelationType: request.Query.Step.RelationType,
				Role:         request.Query.Step.Role,
				Direction:    request.Query.Step.Direction,
			},
			Limit: retrieval.DefaultTraversePageLimit,
		}
		for {
			prePageContinuation := laneContinuation
			pageRequest.Continuation = laneContinuation
			relPage, err := c.Projection.RelationsAtContext(ctx, repo, commit, pageRequest)
			if err != nil {
				return retrieval.TraversePage{}, err
			}
			limitHit := false
			for _, hit := range relPage.Hits {
				edgeKey := retrieval.EdgeCoordinate{Repository: hit.Repository, ObjectID: hit.ObjectID}
				if _, seen := edgesSeen[edgeKey]; seen {
					continue
				}
				edgesSeen[edgeKey] = struct{}{}
				out.Edges = append(out.Edges, retrieval.TraverseEdge{
					Repository: hit.Repository, Commit: hit.Commit, ObjectID: hit.ObjectID,
					Relation: hit.Relation, Depth: entry.Depth,
				})
				for _, endpoint := range hit.Relation.Endpoints {
					if endpoint.ObjectRef == nodeRef {
						continue
					}
					allowed, err := scope.Contains(endpoint.ObjectRef.Repository, endpoint.ObjectRef.Object)
					if err != nil {
						return retrieval.TraversePage{}, err
					}
					if !allowed {
						boundary := retrieval.TraverseBoundary{
							Repository: endpoint.ObjectRef.Repository, ObjectID: endpoint.ObjectRef.Object,
							Reason: retrieval.BoundaryOutsideScope,
						}
						if _, seen := boundarySeen[boundary]; !seen {
							boundarySeen[boundary] = struct{}{}
							position.Boundary = append(position.Boundary, boundary)
							out.Boundary = append(out.Boundary, boundary)
						}
						continue
					}
					if _, seen := visited[endpoint.ObjectRef]; seen {
						continue
					}
					visited[endpoint.ObjectRef] = struct{}{}
					depth := entry.Depth + 1
					frontier = append(frontier, retrieval.TraverseFrontierEntry{
						Repository: endpoint.ObjectRef.Repository, Object: endpoint.ObjectRef.Object, Depth: depth,
					})
					if depth >= request.Query.MinHops {
						out.Nodes = append(out.Nodes, retrieval.TraverseNode{
							Repository: endpoint.ObjectRef.Repository, ObjectID: endpoint.ObjectRef.Object, Depth: depth,
						})
					}
				}
				if len(out.Nodes) >= request.Limit {
					limitHit = true
					break
				}
			}
			for _, claim := range relPage.Claims {
				appendClaim(claim)
			}
			if limitHit {
				// The page limit was reached inside this relation lane page.
				// Rewind to the page start so the next run re-consumes it;
				// seen sets make the re-scan idempotent, so deltas stay
				// disjoint and no relation is lost. The entry returns to the
				// front of the frontier because its expansion is incomplete.
				frontier = append([]retrieval.TraverseFrontierEntry{entry}, frontier...)
				if prePageContinuation == "" {
					delete(positions, key)
				} else {
					positions[key] = prePageContinuation
				}
				if err := pageDone(); err != nil {
					return retrieval.TraversePage{}, err
				}
				return out, nil
			}
			if len(edgesSeen) >= retrieval.MaxTraverseEdges {
				budgetStop("traverse edge budget exhausted")
				if relPage.Exhausted {
					delete(positions, key)
				} else {
					positions[key] = relPage.Continuation
					frontier = append([]retrieval.TraverseFrontierEntry{entry}, frontier...)
				}
				if err := pageDone(); err != nil {
					return retrieval.TraversePage{}, err
				}
				return out, nil
			}
			if relPage.Exhausted {
				delete(positions, key)
				break
			}
			if relPage.Continuation == laneContinuation {
				if len(relPage.Claims) > 0 {
					// The relation lane stopped for its own execution budget
					// and rewound to the pre-page position. Pause this
					// traversal page with the entry pending: the caller
					// resumes with a fresh Execute and fresh lane budget.
					frontier = append([]retrieval.TraverseFrontierEntry{entry}, frontier...)
					if err := pageDone(); err != nil {
						return retrieval.TraversePage{}, err
					}
					return out, nil
				}
				return retrieval.TraversePage{}, kernel.Fail(kernel.ErrPreconditionFailed,
					"traverse relation lane returned a non-advancing continuation")
			}
			laneContinuation = relPage.Continuation
			positions[key] = laneContinuation
		}
	}
	position.Frontier = nil
	position.Positions = positions
	out.Exhausted = true
	out.Boundary = sortBoundaries(out.Boundary)
	return out, nil
}

func refsOf(visited map[knowledge.KnowledgeRef]struct{}) []knowledge.KnowledgeRef {
	refs := make([]knowledge.KnowledgeRef, 0, len(visited))
	for ref := range visited {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Repository != refs[j].Repository {
			return refs[i].Repository < refs[j].Repository
		}
		return refs[i].Object < refs[j].Object
	})
	return refs
}

func edgeRefsOf(edges map[retrieval.EdgeCoordinate]struct{}) []retrieval.EdgeCoordinate {
	list := make([]retrieval.EdgeCoordinate, 0, len(edges))
	for edge := range edges {
		list = append(list, edge)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Repository != list[j].Repository {
			return list[i].Repository < list[j].Repository
		}
		return list[i].ObjectID < list[j].ObjectID
	})
	return list
}

func boundaryKeys(boundary map[retrieval.TraverseBoundary]struct{}) []retrieval.TraverseBoundary {
	list := make([]retrieval.TraverseBoundary, 0, len(boundary))
	for item := range boundary {
		list = append(list, item)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Repository != list[j].Repository {
			return list[i].Repository < list[j].Repository
		}
		if list[i].ObjectID != list[j].ObjectID {
			return list[i].ObjectID < list[j].ObjectID
		}
		return list[i].Reason < list[j].Reason
	})
	return list
}

func sortBoundaries(boundaries []retrieval.TraverseBoundary) []retrieval.TraverseBoundary {
	sort.Slice(boundaries, func(i, j int) bool {
		if boundaries[i].Repository != boundaries[j].Repository {
			return boundaries[i].Repository < boundaries[j].Repository
		}
		if boundaries[i].ObjectID != boundaries[j].ObjectID {
			return boundaries[i].ObjectID < boundaries[j].ObjectID
		}
		return boundaries[i].Reason < boundaries[j].Reason
	})
	return boundaries
}

// DatasetTraverseExecutor runs the traversal inside a named Dataset's fixed
// member scope. The pin is resolved once per request; every hop stays on the
// pinned member commits and the manifest decides candidate eligibility.
type DatasetTraverseExecutor struct {
	Serving      *reader.Serving
	Repositories RepositoryLookup
	Projection   TraverseProjection
}

// datasetFanoutProjection reuses the Dataset relation lane for every hop: a
// relation object may be stored in any member repository, so each expansion
// fans out across the fixed members exactly like one-hop RELATIONS does. The
// fanout page carries its own scope-bound continuation; the traversal core
// persists it opaquely per node.
type datasetFanoutProjection struct {
	inner DatasetRelationsExecutor
}

func (p datasetFanoutProjection) RelationsAtContext(ctx context.Context, _ knowledge.Repository, _ kernel.CommitID, req retrieval.RelationPageRequest) (retrieval.RelationPage, error) {
	return p.inner.Execute(ctx, req)
}

func (e DatasetTraverseExecutor) Execute(ctx context.Context, request retrieval.TraverseRequest) (retrieval.TraversePage, error) {
	if e.Serving == nil || e.Repositories == nil || e.Projection == nil {
		return retrieval.TraversePage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset traverse services are incomplete")
	}
	request, err := retrieval.ValidateTraverseRequest(request)
	if err != nil {
		return retrieval.TraversePage{}, err
	}
	pin := e.Serving.Pin()
	allowed, err := e.Serving.Contains(request.Query.Start.Repository, request.Query.Start.Object)
	if err != nil {
		return retrieval.TraversePage{}, err
	}
	if !allowed {
		return retrieval.TraversePage{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "traverse start is outside dataset")
	}
	ctx, cancel := index.WithSearchBudget(ctx, index.SearchBudget{})
	defer cancel()
	ctx = index.WithCandidateScope(ctx, kernel.CanonicalDigest(pin.Items), func(id kernel.RepositoryID, at kernel.CommitID, object knowledge.ObjectID) (bool, error) {
		if pin.Repositories[id] != at {
			return false, kernel.Fail(kernel.ErrPreconditionFailed, "relation basis is outside dataset")
		}
		return e.Serving.Contains(id, object)
	})
	core := TraverseCore{Repositories: e.Repositories, Projection: datasetFanoutProjection{inner: DatasetRelationsExecutor{
		Serving: e.Serving, Repositories: e.Repositories, Projection: e.Projection,
	}}}
	return core.Run(ctx, request, TraverseScope{
		Snapshots: pin.Repositories, Contains: e.Serving.Contains,
		Identity: string(kernel.CanonicalDigest(pin.Items)),
	})
}

// RepoTraverseExecutor runs the traversal inside one repository at a fixed
// commit. Cross-repository endpoints stop the traversal as explicit
// boundaries; they never widen the scope.
type RepoTraverseExecutor struct {
	Repositories RepositoryLookup
	Projection   TraverseProjection
}

func (e RepoTraverseExecutor) Execute(ctx context.Context, request retrieval.TraverseRequest, repository kernel.RepositoryID, commit kernel.CommitID) (retrieval.TraversePage, error) {
	if e.Repositories == nil || e.Projection == nil {
		return retrieval.TraversePage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository traverse services are incomplete")
	}
	if repository == "" || commit == "" {
		return retrieval.TraversePage{}, kernel.Fail(kernel.ErrUsageInvalid, "repository traverse requires a repository and fixed commit")
	}
	request, err := retrieval.ValidateTraverseRequest(request)
	if err != nil {
		return retrieval.TraversePage{}, err
	}
	scope := TraverseScope{
		Snapshots: map[kernel.RepositoryID]kernel.CommitID{repository: commit},
		Contains: func(id kernel.RepositoryID, _ knowledge.ObjectID) (bool, error) {
			return id == repository, nil
		},
		Identity: "repository:" + string(commit),
	}
	return TraverseCore(e).Run(ctx, request, scope)
}
