package discovery_test

import (
	"testing"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestResidualNonApplicableFieldDoesNotGrantQueryCapability(t *testing.T) {
	repo, at := declarationBuildFixture(t, "skip")
	spec, err := retrieval.AccessSpecForRepository(repo, at)
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read("item/full", at)
	if err != nil {
		t.Fatal(err)
	}
	field := &retrieval.FieldRef{Schema: "schema/item", Path: "name"}
	for _, op := range []retrieval.SearchOp{retrieval.OpMissing, retrieval.OpExists, retrieval.OpEQ} {
		request := retrieval.SearchOf(retrieval.SearchClause{Op: op, Field: field, Value: "GMV"})
		if _, err := retrieval.ResolveSearch(request, spec); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
			t.Fatalf("text-only field gained public %s capability: %v", op, err)
		}
		matched, err := retrieval.MatchesResidual(repo, value, nil, request, spec)
		if err != nil || matched {
			t.Fatalf("nonapplicable %s predicate did not evaluate false: matched=%v error=%v", op, matched, err)
		}
	}
}

func TestResidualUsesConstructedKeywordScalarType(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://test/residual-keyword")
	base := testkit.MustHead(t, repo, snapshot.DefaultRef)
	at, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/count"}, Value: map[string]any{"entity": "Counter", "fields": map[string]any{"count": map[string]any{"type": "integer"}}}},
			testkit.IndexField("schema/count", "count", "keyword"),
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "counter/ten"}, SchemaRef: "schema/count", Value: map[string]any{"count": 10}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := retrieval.AccessSpecForRepository(repo, at)
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read("counter/ten", at)
	if err != nil {
		t.Fatal(err)
	}
	for query, want := range map[string]bool{"10": true, "010": false} {
		request, err := retrieval.ResolveSearch(retrieval.SearchOf(retrieval.SearchEQ("count", query)), spec)
		if err != nil {
			t.Fatal(err)
		}
		matched, err := retrieval.MatchesResidual(repo, value, nil, request, spec)
		if err != nil || matched != want {
			t.Fatalf("keyword count compared as structural integer for %q: got=%v want=%v error=%v", query, matched, want, err)
		}
	}
}
