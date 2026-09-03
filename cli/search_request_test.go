package cli

import (
	"testing"

	"kc/kernel"
	"kc/retrieval"
)

func TestSearchRequestFromFlags(t *testing.T) {
	req, err := searchRequestFromFlags(map[string]FlagValue{
		"query":        "runbook",
		"eq":           "db=tl",
		"in":           "owner=a,b",
		"exists":       "db",
		"gt":           "n=1",
		"sort":         "when:desc",
		"match":        "note=events",
		"missing":      "deleted_at",
		"prefix":       "name=customer.",
		"contains":     "name=tomer",
		"match-mode":   "AnyTerms",
		"continuation": "opaque-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Clauses) != 10 {
		t.Fatalf("%#v", req.Clauses)
	}
	if req.Clauses[0].Op != "MATCH" || req.Clauses[0].Path != "" {
		t.Fatalf("%#v", req.Clauses[0])
	}
	if req.Clauses[9].Op != "MATCH" || req.Clauses[9].Path != "note" || req.Clauses[9].Mode != "AnyTerms" {
		t.Fatalf("%#v", req.Clauses[9])
	}
	if req.Continuation != "opaque-token" {
		t.Fatalf("continuation: %#v", req)
	}
}

func TestSearchRequestParsesExplicitFieldRef(t *testing.T) {
	req, err := searchRequestFromFlags(map[string]FlagValue{
		"eq": "schema/table.owner::owner::name=alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	field := req.Clauses[0].Field
	if field == nil || field.Schema != "schema/table.owner" || field.Aspect != "owner" || field.Path != "name" || req.Clauses[0].Path != "" {
		t.Fatalf("%#v", req.Clauses[0])
	}
}

func TestHTTPKnowledgeSearchRequestPreservesEveryPublicOperator(t *testing.T) {
	wire := knowledgeSearchRequest{
		Workspace: "agent", In: []string{"owner=a,b"}, Exists: []string{"active"},
		Missing: []string{"deleted"}, Prefix: []string{"name=customer."}, Contains: []string{"name=tomer"},
		GreaterThan: []string{"score=1"}, GreaterEqual: []string{"score=2"},
		LessThan: []string{"score=9"}, LessEqual: []string{"score=8"},
	}
	req, err := searchRequestFromFlags(wire.flags())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"IN", "EXISTS", "MISSING", "PREFIX", "CONTAINS", "GT", "GTE", "LT", "LTE"}
	if len(req.Clauses) != len(want) {
		t.Fatalf("clauses = %#v", req.Clauses)
	}
	for i, op := range want {
		if string(req.Clauses[i].Op) != op {
			t.Fatalf("clause %d = %#v, want %s", i, req.Clauses[i], op)
		}
	}
}

func TestHTTPKnowledgeSearchRequestPreservesExpressionAndOrder(t *testing.T) {
	expression := retrieval.SearchAll(
		retrieval.SearchAny(
			retrieval.SearchLeaf(retrieval.SearchMATCH("payment")),
			retrieval.SearchLeaf(retrieval.SearchMATCH("database")),
		),
		retrieval.SearchLeaf(retrieval.SearchEQ("team", "payments")),
	)
	order := retrieval.SearchSORT("severity", "asc")
	wire := knowledgeSearchRequest{Workspace: "agent", Expression: &expression, Order: &order, Limit: 1}
	req, err := wire.searchRequest()
	if err != nil {
		t.Fatal(err)
	}
	if req.Expression == nil || len(req.Expression.All) != 2 || req.Sort == nil || req.Sort.Path != "severity" || req.Limit != 1 {
		t.Fatalf("request = %#v", req)
	}

	wire.Query = "legacy-must-not-mix"
	if _, err := wire.searchRequest(); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("mixed legacy/expression request must fail: %v", err)
	}
}
