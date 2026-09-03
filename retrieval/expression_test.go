package retrieval_test

import (
	"encoding/json"
	"testing"

	"kc/kernel"
	"kc/knowledge/reader"
	"kc/retrieval"
)

func expressionAccessSpec() retrieval.AccessSpec {
	return retrieval.AccessSpecFromReport(reader.SchemaReport{Schemas: []reader.SchemaDescription{{
		ObjectID: "schema/t", Aspect: "structure",
		Fields: []reader.FieldAccess{
			{Path: "db", Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "owner", Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "updated_at", Type: "date", Access: []reader.AccessHint{reader.HintSort}},
		},
	}}})
}

func TestSearchExpressionResolvesAllAnyAndIndependentSort(t *testing.T) {
	req := retrieval.SearchWhere(retrieval.SearchAll(
		retrieval.SearchAny(
			retrieval.SearchLeaf(retrieval.SearchEQ("db", "tl")),
			retrieval.SearchLeaf(retrieval.SearchEQ("db", "prod")),
		),
		retrieval.SearchLeaf(retrieval.SearchEXISTS("owner")),
	))
	sort := retrieval.SearchSORT("updated_at", "desc")
	req.Sort = &sort

	resolved, err := retrieval.ResolveSearch(req, expressionAccessSpec())
	if err != nil {
		t.Fatal(err)
	}
	clauses := retrieval.SearchClauses(resolved)
	if len(clauses) != 4 || clauses[0].Value != "tl" || clauses[1].Value != "prod" || clauses[2].Op != retrieval.OpExists || clauses[3].Op != retrieval.OpSort {
		t.Fatalf("resolved clauses = %#v", clauses)
	}
	for _, clause := range clauses {
		if clause.Field == nil || clause.Path != clause.Field.Key() {
			t.Fatalf("clause was not resolved to a complete field identity: %#v", clause)
		}
	}
	if resolved.Limit != retrieval.DefaultSearchLimit {
		t.Fatalf("default limit = %d", resolved.Limit)
	}
}

func TestSearchExpressionResolvesContainsLeaf(t *testing.T) {
	req := retrieval.SearchWhere(retrieval.SearchAll(
		retrieval.SearchAny(
			retrieval.SearchLeaf(retrieval.SearchCONTAINS("db", "l")),
			retrieval.SearchLeaf(retrieval.SearchEQ("db", "prod")),
		),
		retrieval.SearchLeaf(retrieval.SearchEQ("owner", "alice")),
	))
	resolved, err := retrieval.ResolveSearch(req, expressionAccessSpec())
	if err != nil {
		t.Fatal(err)
	}
	clauses := retrieval.SearchClauses(resolved)
	if len(clauses) != 3 || clauses[0].Op != retrieval.OpContains || clauses[0].Value != "l" || clauses[1].Op != retrieval.OpEQ || clauses[2].Op != retrieval.OpEQ {
		t.Fatalf("resolved clauses = %#v", clauses)
	}
}

func TestSearchExpressionRejectsAmbiguousShapes(t *testing.T) {
	leaf := retrieval.SearchLeaf(retrieval.SearchEQ("db", "tl"))
	for name, req := range map[string]retrieval.SearchRequest{
		"mixed legacy and expression": {
			Clauses: []retrieval.SearchClause{retrieval.SearchEQ("db", "tl")}, Expression: &leaf,
		},
		"empty any": retrieval.SearchWhere(retrieval.SearchAny()),
		"multiple variants": retrieval.SearchWhere(retrieval.SearchExpr{
			Clause: leaf.Clause, All: []retrieval.SearchExpr{leaf},
		}),
		"sort leaf": retrieval.SearchWhere(retrieval.SearchLeaf(retrieval.SearchSORT("updated_at", "asc"))),
	} {
		t.Run(name, func(t *testing.T) {
			if code := kernel.CodeOf(retrieval.ValidateSearch(req)); code != kernel.ErrUsageInvalid {
				t.Fatalf("code = %s", code)
			}
		})
	}
}

func TestSearchExpressionBoundsDepthAndLeaves(t *testing.T) {
	tooDeep := retrieval.SearchLeaf(retrieval.SearchEQ("db", "tl"))
	for range retrieval.MaxSearchExpressionDepth {
		tooDeep = retrieval.SearchAll(tooDeep)
	}
	if code := kernel.CodeOf(retrieval.ValidateSearch(retrieval.SearchWhere(tooDeep))); code != kernel.ErrUsageInvalid {
		t.Fatalf("over-depth expression code = %s", code)
	}

	leaves := make([]retrieval.SearchExpr, retrieval.MaxSearchExpressionLeaves+1)
	for i := range leaves {
		leaves[i] = retrieval.SearchLeaf(retrieval.SearchEQ("db", "tl"))
	}
	if code := kernel.CodeOf(retrieval.ValidateSearch(retrieval.SearchWhere(retrieval.SearchAny(leaves...)))); code != kernel.ErrUsageInvalid {
		t.Fatalf("over-leaf expression code = %s", code)
	}
}

func TestSearchExpressionJSONAndDigestPreserveGrouping(t *testing.T) {
	left := retrieval.SearchWhere(retrieval.SearchAll(
		retrieval.SearchLeaf(retrieval.SearchEQ("db", "tl")),
		retrieval.SearchLeaf(retrieval.SearchEQ("owner", "alice")),
	))
	right := retrieval.SearchWhere(retrieval.SearchAny(
		retrieval.SearchLeaf(retrieval.SearchEQ("db", "tl")),
		retrieval.SearchLeaf(retrieval.SearchEQ("owner", "alice")),
	))
	if retrieval.SearchQueryDigest(left) == retrieval.SearchQueryDigest(right) {
		t.Fatal("continuation query identity must bind All/Any grouping")
	}
	body, err := json.Marshal(right)
	if err != nil {
		t.Fatal(err)
	}
	var decoded retrieval.SearchRequest
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := retrieval.ValidateSearch(decoded); err != nil || decoded.Expression == nil || len(decoded.Expression.Any) != 2 {
		t.Fatalf("decoded = %#v err=%v", decoded, err)
	}
}
