package projection_test

import (
	"context"
	ports "kc/discovery/projection"
	"reflect"
	"testing"

	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

type advancingCandidateEngine struct {
	staleCandidateEngine
	beforeReturn func(ports.RetrieveRequest, ports.CandidatePage)
}

func (e *advancingCandidateEngine) Retrieve(request ports.RetrieveRequest) (ports.CandidatePage, error) {
	page, err := e.staleCandidateEngine.Retrieve(request)
	if err == nil && e.beforeReturn != nil {
		e.beforeReturn(request, page)
	}
	return page, err
}

type fixedBasisAuthorityHydrator struct {
	beforeRead func(objectmodel.Repository, kernel.CommitID, []objectmodel.ObjectID)
}

func (h fixedBasisAuthorityHydrator) ReadMany(repo objectmodel.Repository, commit kernel.CommitID, ids []objectmodel.ObjectID) (map[objectmodel.ObjectID]objectmodel.ObjectValue, error) {
	h.beforeRead(repo, commit, ids)
	values := make(map[objectmodel.ObjectID]objectmodel.ObjectValue, len(ids))
	for _, id := range ids {
		value, err := repo.Read(id, commit)
		if err != nil {
			return nil, err
		}
		values[id] = value
	}
	return values, nil
}

func (h fixedBasisAuthorityHydrator) ReadAddress(repo objectmodel.Repository, commit kernel.CommitID, address objectmodel.Address) (objectmodel.ObjectValue, error) {
	return repo.ReadAddress(address, commit)
}

func TestSearchKeepsCandidateBasisWhenWriterAdvancesBeforeObjectRead(t *testing.T) {
	s := testkit.NewSetup(t, "kr://regression/search-fixed-basis")
	schemaAddress := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}
	article := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/original"}
	schema := func(access string) map[string]any {
		return map[string]any{"entity": "Article", "fields": map[string]any{
			"body":  map[string]any{"type": "string", "access": []any{access}},
			"stage": map[string]any{"type": "string"},
		}}
	}
	commit := func(command string, base kernel.CommitID, operations ...objectmodel.Operation) kernel.CommitID {
		t.Helper()
		receipt, err := s.Writer.Commit(command, objectmodel.CommitChangeSet{
			TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
			BaseCommit: base, ExpectedTargetCommit: base, Operations: operations,
		})
		if err != nil {
			t.Fatal(err)
		}
		return receipt.Result.CommitID
	}
	originalValue := map[string]any{"body": "runbook original", "stage": "a"}
	a := commit("publish-search-a", s.RootCommitID,
		objectmodel.Operation{Op: objectmodel.OpPut, Address: schemaAddress, Value: schema("text")},
		objectmodel.Operation{Op: objectmodel.OpPut, Address: article, SchemaRef: string(schemaAddress.ObjectID), Value: originalValue},
	)
	engine := &advancingCandidateEngine{staleCandidateEngine: staleCandidateEngine{
		candidates: []ports.CandidateRef{{Repository: s.RepositoryID, ObjectID: article.ObjectID, Basis: a}},
	}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(s.Repo, a); err != nil {
		t.Fatal(err)
	}
	specA, err := idx.AccessSpecAt(s.Repo, a)
	if err != nil {
		t.Fatal(err)
	}
	var b kernel.CommitID
	var order []string
	engine.beforeReturn = func(request ports.RetrieveRequest, page ports.CandidatePage) {
		if request.Spec.Commit != a || request.Spec.AccessDigest != specA.AccessDigest ||
			len(page.Candidates) != 1 || page.Candidates[0].Basis != a {
			t.Fatalf("candidate source did not use a's declaration and coordinates: request=%#v page=%#v", request, page)
		}
		order = append(order, "candidate:a")
		b = commit("advance-during-search-b", a,
			objectmodel.Operation{Op: objectmodel.OpPut, Address: schemaAddress, Value: schema("filter")},
			objectmodel.Operation{Op: objectmodel.OpPut, Address: article, SchemaRef: string(schemaAddress.ObjectID), Value: map[string]any{"body": "retired content", "stage": "b"}},
			objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/new"}, SchemaRef: string(schemaAddress.ObjectID), Value: map[string]any{"body": "runbook new", "stage": "b"}},
		)
		order = append(order, "writer:b")
	}
	idx.SetHydrator(fixedBasisAuthorityHydrator{beforeRead: func(repo objectmodel.Repository, basis kernel.CommitID, ids []objectmodel.ObjectID) {
		if b == "" || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != b || basis != a ||
			repo.ID() != s.RepositoryID || !reflect.DeepEqual(ids, []objectmodel.ObjectID{article.ObjectID}) {
			t.Fatalf("object read did not occur after b at a's exact basis: basis=%s ids=%v b=%s", basis, ids, b)
		}
		order = append(order, "authority-read:a")
	}})
	result, err := idx.SearchAtContext(context.Background(), s.Repo, a, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"candidate:a", "writer:b", "authority-read:a"}) ||
		result.Completeness != retrieval.CompletenessComplete || result.Continuation != "" || len(result.Hits) != 1 ||
		!reflect.DeepEqual(result.SearchView.Snapshots, map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: a}) {
		t.Fatalf("search changed its candidate set, completeness, or fixed view: order=%v result=%#v", order, result)
	}
	hit := result.Hits[0]
	if hit.Object.Address != article || hit.Object.Commit != a || hit.Version.DeclarationCommit != a ||
		!reflect.DeepEqual(hit.Object.Value, originalValue) || len(hit.Version.Units) != 1 ||
		hit.Version.Units[0].SchemaRef != string(schemaAddress.ObjectID) || hit.Version.Units[0].ValueBasis != "snapshot:"+string(a) {
		t.Fatalf("candidate a was combined with later content or version: %#v", hit)
	}
	// Prove the concurrent Writer change was material: HEAD, object value,
	// Schema access and object membership all differ at b.
	latest, err := s.Repo.Read(article.ObjectID, b)
	if err != nil || latest.Value.(map[string]any)["stage"] != "b" || reflect.DeepEqual(latest.Value, originalValue) {
		t.Fatalf("b did not replace the object: %#v %v", latest, err)
	}
	latestSchema, err := s.Repo.Read(schemaAddress.ObjectID, b)
	if err != nil || !reflect.DeepEqual(latestSchema.Value, schema("filter")) {
		t.Fatalf("b did not replace the Schema: %#v %v", latestSchema, err)
	}
	specB, err := retrieval.AccessSpecFromAdoptions(s.Repo, b, nil)
	if err != nil || specB.AccessDigest == specA.AccessDigest {
		t.Fatalf("Schema change did not alter the query contract: old=%#v new=%#v error=%v", specA, specB, err)
	}
	added, err := s.Repo.Read("article/new", b)
	if err != nil || added.Value.(map[string]any)["body"] != "runbook new" {
		t.Fatalf("b did not add the competing object: %#v %v", added, err)
	}
}
