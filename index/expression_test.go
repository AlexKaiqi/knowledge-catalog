package index_test

import (
	"testing"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
	"kc/snapshot"
)

func (*supersetEngine) ProbeExpression(retrieval.SearchExpr, retrieval.AccessSpec) index.Capability {
	return index.Capability{Guarantee: index.GuaranteeSuperset, Coverage: 1}
}

func TestSupersetResidualEvaluatesNestedAllAnyExpression(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/expression-residual")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "fields": map[string]any{
				"db":    map[string]any{"type": "string", "access": []any{"filter"}},
				"owner": map[string]any{"type": "string", "access": []any{"filter"}},
			},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:no", AspectName: "structure"}, Value: map[string]any{"db": "other", "owner": "alice"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:a", AspectName: "structure"}, Value: map[string]any{"db": "tl", "owner": "alice"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:b", AspectName: "structure"}, Value: map[string]any{"db": "prod", "owner": "bob"}},
	})
	engine := &supersetEngine{ids: []knowledge.ObjectID{"Table:no", "Table:a", "Table:b"}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
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
	if result.Completeness != retrieval.CompletenessComplete || len(result.Hits) != 1 || result.Hits[0].Knowledge.Address.ObjectID != "Table:a" {
		t.Fatalf("result = %#v", result)
	}
}

func TestExpressionSearchFailsClosedWithoutCompositionProof(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/expression-proof")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "fields": map[string]any{
				"db": map[string]any{"type": "string", "access": []any{"filter"}},
			},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:a", AspectName: "structure"}, Value: map[string]any{"db": "tl"}},
	})
	engine := &staleCandidateEngine{}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
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
