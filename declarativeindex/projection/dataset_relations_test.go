package projection_test

import (
	"context"
	"testing"

	"kc/declarativeindex/internal/testfixture"
	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type datasetRelationEngine struct {
	datasetTestEngine
	relationQueries int
}

func (e *datasetRelationEngine) RetrieveRelations(request retrieval.RelationRetrieveRequest) (retrieval.RelationCandidatePage, error) {
	e.relationQueries++
	page := retrieval.RelationCandidatePage{Exhausted: true}
	for _, doc := range e.docs {
		if doc.Relation == nil {
			continue
		}
		for _, endpoint := range doc.Relation.Endpoints {
			if endpoint.Object == request.Query.Endpoint {
				page.Candidates = append(page.Candidates, retrieval.RelationCandidate{Repository: request.Repository, Basis: request.Basis, ObjectID: doc.ObjectID})
				break
			}
		}
	}
	return page, nil
}

func TestDatasetRelationsUseOneCorpusAndReadOnlySelectedFixedSources(t *testing.T) {
	repos := map[kernel.RepositoryID]*testkit.ObjectRepository{}
	selectedIDs := map[kernel.RepositoryID]objectmodel.ObjectID{}
	pin := reader.DatasetPin{SetID: "relations", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{}}
	for _, name := range []string{"one", "two"} {
		repo := testkit.MakeRepository(t, "kr://dataset/relations/"+name)
		base, _ := repo.Head(snapshot.DefaultRef)
		selected, private := objectmodel.ObjectID("relation/"+name), objectmodel.ObjectID("relation/private-"+name)
		at, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: selected}, PathHint: "selected/" + name + ".yaml", Value: testfixture.RelationBody(string(selected), "table/a")},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: private}, PathHint: "private/" + name + ".yaml", Value: testfixture.RelationBody(string(private), "table/a")},
		}})
		if err != nil {
			t.Fatal(err)
		}
		repos[repo.ID()] = repo
		selectedIDs[repo.ID()] = selected
		pin.Repositories[repo.ID()] = at
		pin.Items = append(pin.Items, reader.DatasetItem{Repository: repo.ID(), Commit: at, Kind: "prefix", Prefix: "selected"})
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) { return repos[id].Repository, nil }
	engines := map[kernel.RepositoryID]*datasetRelationEngine{}
	idx := index.NewIndexEngine(t.TempDir(), func(_ string, id kernel.RepositoryID) (ports.Engine, error) {
		if engines[id] == nil {
			engines[id] = &datasetRelationEngine{}
		}
		return engines[id], nil
	})
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
		t.Fatal(err)
	}
	for id, repo := range repos {
		at := pin.Repositories[id]
		if _, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: id, TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: []objectmodel.Operation{
			{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: selectedIDs[id]}},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := idx.RelationsDataset(context.Background(), lookup, pin, relationRequest())
	if err != nil || len(page.Hits) != 2 || !page.Exhausted {
		t.Fatalf("Dataset relation corpus did not retain both fixed selected sources: %#v %v", page, err)
	}
	for _, hit := range page.Hits {
		if hit.Commit != pin.Repositories[hit.Repository] || hit.Relation.RelationID != hit.ObjectID || hit.ObjectID != "relation/one" && hit.ObjectID != "relation/two" {
			t.Fatalf("relation hit lost its stored source coordinate or leaked a private relation: %#v", hit)
		}
	}
	queries := 0
	for _, engine := range engines {
		queries += engine.relationQueries
	}
	if len(engines) != 1 || queries != 1 {
		t.Fatalf("Dataset relations fanned out over sources: engines=%d queries=%d", len(engines), queries)
	}
}
