package projection_test

import (
	"kc/declarativeindex/internal/testfixture"
	ports "kc/discovery/projection"
	"testing"

	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func (*supersetEngine) ProbeExpression(retrieval.SearchExpr, retrieval.AccessSpec) ports.Capability {
	return ports.Capability{Guarantee: ports.GuaranteeSuperset, Coverage: 1}
}

func TestSupersetResidualEvaluatesNestedAllAnyExpression(t *testing.T) {
	repo := testfixture.MakeIndexRepository(t, "kr://acme/public/expression-residual")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := testfixture.PutAt(t, repo, root, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "fields": map[string]any{
				"db":    map[string]any{"type": "string"},
				"owner": map[string]any{"type": "string"},
			},
		}},
		testkit.IndexField("schema/table.structure", "db", "keyword"),
		testkit.IndexField("schema/table.structure", "owner", "keyword"),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/no", AspectName: "structure"}, Value: map[string]any{"db": "other", "owner": "alice"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/a", AspectName: "structure"}, Value: map[string]any{"db": "tl", "owner": "alice"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/b", AspectName: "structure"}, Value: map[string]any{"db": "prod", "owner": "bob"}},
	})
	engine := &supersetEngine{ids: []objectmodel.ObjectID{"table/no", "table/a", "table/b"}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	request := retrieval.SearchWhere(retrieval.SearchAll(
		retrieval.SearchAny(
			retrieval.SearchLeaf(retrieval.SearchEQ("db", "tl")),
			retrieval.SearchLeaf(retrieval.SearchEQ("db", "prod")),
		),
		retrieval.SearchLeaf(retrieval.SearchEQ("owner", "alice")),
	))
	result, err := idx.SearchAt(repo, head, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Completeness != retrieval.CompletenessComplete || len(result.Hits) != 1 || result.Hits[0].Object.Address.ObjectID != "table/a" {
		t.Fatalf("result = %#v", result)
	}
}

func TestSupersetResidualContainsKeepsSubstringHits(t *testing.T) {
	repo := testfixture.MakeIndexRepository(t, "kr://acme/public/contains-residual")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := testfixture.PutAt(t, repo, root, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "fields": map[string]any{
				"db": map[string]any{"type": "string"},
			},
		}},
		testkit.IndexField("schema/table.structure", "db", "keyword"),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/a", AspectName: "structure"}, Value: map[string]any{"db": "tl"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/b", AspectName: "structure"}, Value: map[string]any{"db": "dw"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/c", AspectName: "structure"}, Value: map[string]any{"db": "tl"}},
	})
	engine := &supersetEngine{ids: []objectmodel.ObjectID{"table/a", "table/b", "table/c"}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	result, err := idx.SearchAt(repo, head, retrieval.SearchOf(retrieval.SearchCONTAINS("db", "l")))
	if err != nil {
		t.Fatal(err)
	}
	if result.Completeness != retrieval.CompletenessComplete || len(result.Hits) != 2 {
		t.Fatalf("result = %#v", result)
	}
	seen := map[objectmodel.ObjectID]bool{}
	for _, hit := range result.Hits {
		seen[hit.Object.Address.ObjectID] = true
	}
	if !seen["table/a"] || !seen["table/c"] || seen["table/b"] {
		t.Fatalf("CONTAINS residual must keep substring hits only: %#v", result.Hits)
	}
}

func TestSupersetResidualContainsTreatsWildcardMetaAsLiteral(t *testing.T) {
	repo := testfixture.MakeIndexRepository(t, "kr://acme/public/contains-literal-meta")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := testfixture.PutAt(t, repo, root, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "fields": map[string]any{
				"db": map[string]any{"type": "string"},
			},
		}},
		testkit.IndexField("schema/table.structure", "db", "keyword"),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/star", AspectName: "structure"}, Value: map[string]any{"db": "prod*east"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/dash", AspectName: "structure"}, Value: map[string]any{"db": "prod-east"}},
	})
	engine := &supersetEngine{ids: []objectmodel.ObjectID{"table/star", "table/dash"}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	star, err := idx.SearchAt(repo, head, retrieval.SearchOf(retrieval.SearchCONTAINS("db", "prod*")))
	if err != nil || len(star.Hits) != 1 || star.Hits[0].Object.Address.ObjectID != "table/star" {
		t.Fatalf("literal CONTAINS prod* must not GLOB: %#v %v", star, err)
	}
	both, err := idx.SearchAt(repo, head, retrieval.SearchOf(retrieval.SearchCONTAINS("db", "east")))
	if err != nil || len(both.Hits) != 2 {
		t.Fatalf("substring east: %#v %v", both, err)
	}
}

func TestExpressionSearchFailsClosedWithoutCompositionProof(t *testing.T) {
	repo := testfixture.MakeIndexRepository(t, "kr://acme/public/expression-proof")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := testfixture.PutAt(t, repo, root, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "fields": map[string]any{
				"db": map[string]any{"type": "string"},
			},
		}},
		testkit.IndexField("schema/table.structure", "db", "keyword"),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/a", AspectName: "structure"}, Value: map[string]any{"db": "tl"}},
	})
	engine := &staleCandidateEngine{}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	request := retrieval.SearchWhere(retrieval.SearchAny(
		retrieval.SearchLeaf(retrieval.SearchEQ("db", "tl")),
		retrieval.SearchLeaf(retrieval.SearchEQ("db", "prod")),
	))
	if _, err := idx.SearchAt(repo, head, request); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing composition proof must fail closed: %v", err)
	}
}
