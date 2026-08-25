package cli

import "testing"

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
		"match-mode":   "AnyTerms",
		"continuation": "opaque-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Clauses) != 9 {
		t.Fatalf("%#v", req.Clauses)
	}
	if req.Clauses[0].Op != "MATCH" || req.Clauses[0].Path != "" {
		t.Fatalf("%#v", req.Clauses[0])
	}
	if req.Clauses[8].Op != "MATCH" || req.Clauses[8].Path != "note" || req.Clauses[8].Mode != "AnyTerms" {
		t.Fatalf("%#v", req.Clauses[8])
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
