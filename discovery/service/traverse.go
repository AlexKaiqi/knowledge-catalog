package service

import (
	"context"
	"sort"

	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// Traversal executes the bounded neighborhood closure contract from
// doc/architecture.md: the scope is fixed before execution,
// every hop reuses the one-hop relation execution lane at the pinned basis,
// authorization is checked before each expansion, and pages are disjoint
// deltas whose union is the closure. Nodes are object IDs: each hop asks the
// scope's relation lane for relations touching the ID wherever they are
// stored, and endpoint IDs no Repository in the fixed scope holds stop the
// traversal as explicit boundaries.

// TraverseScope is the fixed expansion range resolved by the calling channel.
// Identity is the canonical scope identity bound into continuation tokens:
// the dataset manifest digest for the dataset channel, the pinned commit for
// the single-repository channel.
type TraverseScope struct {
	Snapshots map[kernel.RepositoryID]kernel.CommitID
	Contains  func(objectmodel.ObjectID) (bool, error)
	Identity  string
}

// TraverseProjection is the per-repository relation execution port: the same
// lane one-hop RELATIONS uses, never an authority scan.
type TraverseProjection interface {
	RelationsAtContext(context.Context, objectmodel.Repository, kernel.CommitID, retrieval.RelationPageRequest) (retrieval.RelationPage, error)
}

// TraverseHop runs one relation page for one node inside the fixed scope.
type TraverseHop func(context.Context, retrieval.RelationPageRequest) (retrieval.RelationPage, error)

// TraverseCore runs one traversal page against a fixed scope.
type TraverseCore struct {
	Hop TraverseHop
}

// Run expands the closure until the page limit, the edge budget, or the
// frontier is exhausted, and returns the delta plus a resumable position.
func (c TraverseCore) Run(ctx context.Context, request retrieval.TraverseRequest, scope TraverseScope) (retrieval.TraversePage, error) {
	if c.Hop == nil {
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
	visited := make(map[objectmodel.ObjectID]struct{}, len(position.Visited))
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
		allowed, err := scope.Contains(start)
		if err != nil {
			return retrieval.TraversePage{}, err
		}
		if !allowed {
			return retrieval.TraversePage{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "traverse start is outside the fixed scope")
		}
		visited[start] = struct{}{}
		frontier = []retrieval.TraverseFrontierEntry{{Object: start, Depth: 0}}
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
		node := entry.Object
		key := retrieval.TraversePositionKey(node)
		laneContinuation := positions[key]
		pageRequest := retrieval.RelationPageRequest{
			Query: retrieval.RelationQuery{
				Endpoint:     node,
				RelationType: request.Query.Step.RelationType,
				Role:         request.Query.Step.Role,
				Direction:    request.Query.Step.Direction,
			},
			Limit: retrieval.DefaultTraversePageLimit,
		}
		for {
			prePageContinuation := laneContinuation
			pageRequest.Continuation = laneContinuation
			relPage, err := c.Hop(ctx, pageRequest)
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
					if endpoint.Object == node {
						continue
					}
					if _, seen := visited[endpoint.Object]; seen {
						continue
					}
					allowed, err := scope.Contains(endpoint.Object)
					if err != nil {
						return retrieval.TraversePage{}, err
					}
					if !allowed {
						boundary := retrieval.TraverseBoundary{ObjectID: endpoint.Object, Reason: retrieval.BoundaryOutsideScope}
						if _, seen := boundarySeen[boundary]; !seen {
							boundarySeen[boundary] = struct{}{}
							position.Boundary = append(position.Boundary, boundary)
							out.Boundary = append(out.Boundary, boundary)
						}
						continue
					}
					visited[endpoint.Object] = struct{}{}
					depth := entry.Depth + 1
					frontier = append(frontier, retrieval.TraverseFrontierEntry{Object: endpoint.Object, Depth: depth})
					if depth >= request.Query.MinHops {
						out.Nodes = append(out.Nodes, retrieval.TraverseNode{ObjectID: endpoint.Object, Depth: depth})
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

func refsOf(visited map[objectmodel.ObjectID]struct{}) []objectmodel.ObjectID {
	refs := make([]objectmodel.ObjectID, 0, len(visited))
	for ref := range visited {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i] < refs[j] })
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
		if list[i].ObjectID != list[j].ObjectID {
			return list[i].ObjectID < list[j].ObjectID
		}
		return list[i].Reason < list[j].Reason
	})
	return list
}

func sortBoundaries(boundaries []retrieval.TraverseBoundary) []retrieval.TraverseBoundary {
	sort.Slice(boundaries, func(i, j int) bool {
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

func (e DatasetTraverseExecutor) Execute(ctx context.Context, request retrieval.TraverseRequest) (retrieval.TraversePage, error) {
	if e.Serving == nil || e.Repositories == nil || e.Projection == nil {
		return retrieval.TraversePage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset traverse services are incomplete")
	}
	request, err := retrieval.ValidateTraverseRequest(request)
	if err != nil {
		return retrieval.TraversePage{}, err
	}
	pin := e.Serving.Pin()
	ctx, cancel := index.WithSearchBudget(ctx, index.SearchBudget{})
	defer cancel()
	ctx = index.WithCandidateScope(ctx, kernel.CanonicalDigest(pin.Items), func(id kernel.RepositoryID, at kernel.CommitID, object objectmodel.ObjectID) (bool, error) {
		if pin.Repositories[id] != at {
			return false, kernel.Fail(kernel.ErrPreconditionFailed, "relation basis is outside dataset")
		}
		return e.Serving.Contains(id, object)
	})
	// A relation object may be stored in any member, so every hop fans out
	// across the fixed members exactly like one-hop RELATIONS does.
	lane := DatasetRelationsExecutor{Serving: e.Serving, Repositories: e.Repositories, Projection: e.Projection}
	return TraverseCore{Hop: lane.Execute}.Run(ctx, request, TraverseScope{
		Snapshots: pin.Repositories, Contains: e.Serving.Holds,
		Identity: string(projectionScopeDigest(pin.Items, e.Projection, pin.Repositories)),
	})
}

// RepoTraverseExecutor runs the traversal over the relations one Repository
// stores at a fixed commit. As in a Dataset, a node is in scope when the
// scope holds its ID; IDs this Repository does not hold are explicit
// boundaries and never widen the scope.
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
	repo, err := e.Repositories.Require(repository, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		return retrieval.TraversePage{}, err
	}
	scope := TraverseScope{
		Snapshots: map[kernel.RepositoryID]kernel.CommitID{repository: commit},
		Contains:  func(id objectmodel.ObjectID) (bool, error) { return holdsObject(repo, commit, id) },
		Identity:  "repository:" + string(commit),
	}
	if selected, ok := e.Projection.(interface {
		SelectionDigest(map[kernel.RepositoryID]kernel.CommitID) kernel.Digest
	}); ok {
		if identity := selected.SelectionDigest(scope.Snapshots); identity != "" {
			scope.Identity += ":" + string(identity)
		}
	}
	hop := func(ctx context.Context, req retrieval.RelationPageRequest) (retrieval.RelationPage, error) {
		return e.Projection.RelationsAtContext(ctx, repo, commit, req)
	}
	return TraverseCore{Hop: hop}.Run(ctx, request, scope)
}

// holdsObject reports whether one Repository stores the object ID at commit.
func holdsObject(repo objectmodel.Repository, commit kernel.CommitID, id objectmodel.ObjectID) (bool, error) {
	locator, ok := repo.(objectmodel.UnitLocator)
	if !ok {
		return false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s cannot locate object units", repo.ID())
	}
	paths, err := locator.ObjectUnitPaths(id, commit)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return len(paths) > 0, nil
}
