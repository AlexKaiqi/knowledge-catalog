package knowledgeapp

import (
	"context"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
)

type traverseTestProjection struct {
	relations map[string][]retrieval.RelationHit
}

func (p *traverseTestProjection) RelationsAtContext(_ context.Context, repo knowledge.Repository, at kernel.CommitID, req retrieval.RelationPageRequest) (retrieval.RelationPage, error) {
	key := string(repo.ID()) + "\x1f" + string(req.Query.Endpoint.Object)
	hits := []retrieval.RelationHit{}
	for _, hit := range p.relations[key] {
		if req.Query.RelationType != "" && hit.Relation.RelationType != req.Query.RelationType {
			continue
		}
		if req.Query.Direction != "" && hit.Relation.Direction != req.Query.Direction {
			continue
		}
		if req.Query.Role != "" {
			matched := false
			for _, endpoint := range hit.Relation.Endpoints {
				if endpoint.ObjectRef == req.Query.Endpoint && endpoint.Role == req.Query.Role {
					matched = true
				}
			}
			if !matched {
				continue
			}
		}
		hits = append(hits, hit)
	}
	return retrieval.RelationPage{Hits: hits, Exhausted: true}, nil
}

func traverseEndpoint(repo kernel.RepositoryID, object string) knowledge.KnowledgeRef {
	return knowledge.KnowledgeRef{Repository: repo, Object: knowledge.ObjectID(object)}
}

func traverseRelation(t *testing.T, repo kernel.RepositoryID, commit kernel.CommitID, id, relationType string, direction knowledge.RelationDirection, endpoints ...[2]string) retrieval.RelationHit {
	t.Helper()
	relation := knowledge.CanonicalRelation{RelationID: knowledge.ObjectID(id), RelationType: relationType, Direction: direction}
	for _, pair := range endpoints {
		relation.Endpoints = append(relation.Endpoints, knowledge.RelationEndpoint{
			Role:      pair[0],
			ObjectRef: traverseRef(pair[1]),
		})
	}
	return retrieval.RelationHit{Repository: repo, Commit: commit, ObjectID: knowledge.ObjectID(id), Relation: relation}
}

// traverseRef decodes "repo|object" coordinates used by the fixture graph.
func traverseRef(encoded string) knowledge.KnowledgeRef {
	for i := 0; i < len(encoded); i++ {
		if encoded[i] == '|' {
			return knowledge.KnowledgeRef{Repository: kernel.RepositoryID(encoded[:i]), Object: knowledge.ObjectID(encoded[i+1:])}
		}
	}
	panic("bad fixture reference " + encoded)
}

func traverseFixture(t *testing.T) (relationTestLookup, reader.KnowledgeSetPin, *traverseTestProjection) {
	t.Helper()
	repos := relationTestLookup{}
	pin := reader.KnowledgeSetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{}}
	for _, id := range []kernel.RepositoryID{"kr://a/entities", "kr://b/graph", "kr://z/graph"} {
		repos[id] = relationTestRepository{id: id}
		pin.Repositories[id] = kernel.CommitID(string(id) + "-fixed")
	}
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	commitA, commitB, commitZ := pin.Repositories["kr://a/entities"], pin.Repositories["kr://b/graph"], pin.Repositories["kr://z/graph"]
	projection := &traverseTestProjection{relations: map[string][]retrieval.RelationHit{
		"kr://a/entities\x1fobject/a": {
			traverseRelation(t, "kr://a/entities", commitA, "relation/a1", "owned-by", knowledge.RelationDirected,
				[2]string{"subject", "kr://a/entities|object/a"}, [2]string{"owner", "kr://b/graph|object/b"}),
			traverseRelation(t, "kr://a/entities", commitA, "relation/a2", "owned-by", knowledge.RelationDirected,
				[2]string{"subject", "kr://a/entities|object/a"}, [2]string{"owner", "kr://z/graph|object/x"}),
		},
		"kr://b/graph\x1fobject/b": {
			traverseRelation(t, "kr://b/graph", commitB, "relation/b1", "owned-by", knowledge.RelationDirected,
				[2]string{"subject", "kr://b/graph|object/b"}, [2]string{"owner", "kr://b/graph|object/b"}),
			traverseRelation(t, "kr://b/graph", commitB, "relation/b2", "owned-by", knowledge.RelationDirected,
				[2]string{"subject", "kr://b/graph|object/b"}, [2]string{"owner", "kr://b/graph|object/c"}),
		},
		"kr://b/graph\x1fobject/c": {
			traverseRelation(t, "kr://b/graph", commitB, "relation/c1", "owned-by", knowledge.RelationDirected,
				[2]string{"subject", "kr://b/graph|object/c"}, [2]string{"owner", "kr://a/entities|object/a"}),
		},
		"kr://z/graph\x1fobject/x": {
			traverseRelation(t, "kr://z/graph", commitZ, "relation/z1", "owned-by", knowledge.RelationDirected,
				[2]string{"subject", "kr://z/graph|object/x"}, [2]string{"owner", "kr://out/repo|object/out"}),
		},
	}}
	return repos, pin, projection
}

func traverseExecutor(t *testing.T) (DatasetTraverseExecutor, reader.KnowledgeSetPin) {
	t.Helper()
	repos, pin, projection := traverseFixture(t)
	lookup := func(id kernel.RepositoryID) (knowledge.Repository, error) {
		return repos.Require(id, kernel.ErrKnowledgeRefUnresolved)
	}
	executor := DatasetTraverseExecutor{Serving: reader.Open(lookup, pin), Repositories: repos, Projection: projection}
	return executor, pin
}

func traverseRequest(start knowledge.KnowledgeRef, maxHops int) retrieval.TraverseRequest {
	return retrieval.TraverseRequest{Query: retrieval.TraverseQuery{Start: start, MaxHops: maxHops}}
}

func traverseCollect(t *testing.T, executor DatasetTraverseExecutor, request retrieval.TraverseRequest) retrieval.TraversePage {
	t.Helper()
	union := retrieval.TraversePage{Nodes: []retrieval.TraverseNode{}, Edges: []retrieval.TraverseEdge{}}
	for pages := 0; ; pages++ {
		if pages > 16 {
			t.Fatal("traverse pagination did not terminate")
		}
		page, err := executor.Execute(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		union.Nodes = append(union.Nodes, page.Nodes...)
		union.Edges = append(union.Edges, page.Edges...)
		union.Boundary = append(union.Boundary, page.Boundary...)
		union.Claims = append(union.Claims, page.Claims...)
		if page.Exhausted {
			break
		}
		if page.Continuation == "" {
			t.Fatal("non-exhausted traverse page without continuation")
		}
		request.Continuation = page.Continuation
	}
	return union
}

func TestTraverseClosureDeduplicatesAndReportsBoundaries(t *testing.T) {
	executor, _ := traverseExecutor(t)
	union := traverseCollect(t, executor, traverseRequest(traverseEndpoint("kr://a/entities", "object/a"), 3))
	if len(union.Nodes) != 3 {
		t.Fatalf("closure nodes: %#v", union.Nodes)
	}
	depths := map[string]int{}
	for _, node := range union.Nodes {
		if node.ObjectID == "object/a" {
			t.Fatal("start must never be a node")
		}
		depths[string(node.ObjectID)] = node.Depth
	}
	if depths["object/b"] != 1 || depths["object/x"] != 1 || depths["object/c"] != 2 {
		t.Fatalf("minimum depths: %#v", depths)
	}
	if len(union.Edges) != 6 {
		t.Fatalf("closure edges: %#v", union.Edges)
	}
	seen := map[string]int{}
	for _, edge := range union.Edges {
		seen[string(edge.ObjectID)]++
		if edge.Relation.RelationID != edge.ObjectID {
			t.Fatalf("edge lost its relation envelope: %#v", edge)
		}
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf("edge %s returned %d times", id, count)
		}
	}
	if len(union.Boundary) != 1 || union.Boundary[0].Repository != "kr://out/repo" ||
		union.Boundary[0].ObjectID != "object/out" || union.Boundary[0].Reason != retrieval.BoundaryOutsideScope {
		t.Fatalf("out-of-scope frontier was not an explicit boundary: %#v", union.Boundary)
	}
}

func TestTraverseHopCeilingAndMinHopsFilter(t *testing.T) {
	executor, _ := traverseExecutor(t)
	request := traverseRequest(traverseEndpoint("kr://a/entities", "object/a"), 1)
	page := traverseCollect(t, executor, request)
	if len(page.Nodes) != 2 || len(page.Edges) != 2 || len(page.Boundary) != 0 {
		t.Fatalf("one-hop closure: %#v", page)
	}
	union := traverseCollect(t, executor, retrieval.TraverseRequest{
		Query: retrieval.TraverseQuery{Start: traverseEndpoint("kr://a/entities", "object/a"), MinHops: 2, MaxHops: 3},
	})
	if len(union.Nodes) != 1 || union.Nodes[0].ObjectID != "object/c" || union.Nodes[0].Depth != 2 {
		t.Fatalf("minHops did not filter the node set: %#v", union.Nodes)
	}
	if len(union.Edges) != 6 {
		t.Fatalf("minHops must not filter edges: %#v", union.Edges)
	}
}

func TestTraverseStartOutsideScopeFailsClosed(t *testing.T) {
	executor, _ := traverseExecutor(t)
	_, err := executor.Execute(context.Background(), traverseRequest(traverseEndpoint("kr://out/repo", "object/out"), 2))
	if kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		t.Fatalf("start outside dataset: %v", err)
	}
}

func TestTraverseContinuationBoundToDatasetScope(t *testing.T) {
	repos, pin, projection := traverseFixture(t)
	lookup := func(id kernel.RepositoryID) (knowledge.Repository, error) {
		return repos.Require(id, kernel.ErrKnowledgeRefUnresolved)
	}
	executor := DatasetTraverseExecutor{Serving: reader.Open(lookup, pin), Repositories: repos, Projection: projection}
	request := traverseRequest(traverseEndpoint("kr://a/entities", "object/a"), 3)
	request.Limit = 1
	first, err := executor.Execute(context.Background(), request)
	if err != nil || first.Continuation == "" || first.Exhausted {
		t.Fatalf("first delta page: %#v %v", first, err)
	}
	request.Continuation = first.Continuation
	pin.Items[0].Target = "changed-layout"
	executor.Serving = reader.Open(lookup, pin)
	if _, err := executor.Execute(context.Background(), request); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("continuation crossed Dataset scope: %v", err)
	}
}

func TestTraverseValidationRejectsUnboundedShapes(t *testing.T) {
	executor, _ := traverseExecutor(t)
	start := traverseEndpoint("kr://a/entities", "object/a")
	for _, request := range []retrieval.TraverseRequest{
		traverseRequest(start, 0),
		traverseRequest(start, retrieval.MaxTraverseHops+1),
		{Query: retrieval.TraverseQuery{Start: start, MaxHops: 3, MinHops: 4}},
		{Query: retrieval.TraverseQuery{Start: traverseEndpoint("", "object/a"), MaxHops: 2}},
		{Query: retrieval.TraverseQuery{Start: start, MaxHops: 2}, Limit: retrieval.MaxTraversePageLimit + 1},
	} {
		if _, err := executor.Execute(context.Background(), request); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("invalid traverse shape accepted: %#v %v", request, err)
		}
	}
}

func TestRepoTraverseStopsAtRepositoryBoundary(t *testing.T) {
	repos, pin, projection := traverseFixture(t)
	executor := RepoTraverseExecutor{Repositories: repos, Projection: projection}
	request := traverseRequest(knowledge.KnowledgeRef{Repository: "kr://a/entities", Object: "object/a"}, 3)
	page, err := executor.Execute(context.Background(), request, "kr://a/entities", pin.Repositories["kr://a/entities"])
	if err != nil {
		t.Fatal(err)
	}
	if !page.Exhausted {
		t.Fatalf("single-repository traversal must terminate: %#v", page)
	}
	// Both neighbors of the start live in other repositories: the edges stay
	// visible (their bodies are this repository's authorized relations), the
	// neighbors are explicit boundaries, and nothing beyond them is expanded.
	if len(page.Nodes) != 0 {
		t.Fatalf("out-of-scope neighbors must not become nodes: %#v", page.Nodes)
	}
	if len(page.Edges) != 2 {
		t.Fatalf("in-scope edges: %#v", page.Edges)
	}
	if len(page.Boundary) != 2 {
		t.Fatalf("cross-repository frontiers must be explicit boundaries: %#v", page.Boundary)
	}
	boundaryIDs := map[string]bool{}
	for _, item := range page.Boundary {
		if item.Reason != retrieval.BoundaryOutsideScope {
			t.Fatalf("unexpected boundary reason: %#v", item)
		}
		boundaryIDs[string(item.ObjectID)] = true
	}
	if !boundaryIDs["object/b"] || !boundaryIDs["object/x"] {
		t.Fatalf("missing boundary entries: %#v", page.Boundary)
	}
}
