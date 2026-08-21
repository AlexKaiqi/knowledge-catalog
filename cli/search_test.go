package cli

import "testing"

func TestSearchRequestFromFlags(t *testing.T) {
	req, err := searchRequestFromFlags(map[string]FlagValue{
		"query":  "runbook",
		"eq":     "db=tl",
		"in":     "owner=a,b",
		"exists": "db",
		"gt":     "n=1",
		"sort":   "when:desc",
		"match":  "note=events",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Clauses) != 7 {
		t.Fatalf("%#v", req.Clauses)
	}
	if req.Clauses[0].Op != "MATCH" || req.Clauses[0].Path != "" {
		t.Fatalf("%#v", req.Clauses[0])
	}
	if req.Clauses[6].Op != "MATCH" || req.Clauses[6].Path != "note" {
		t.Fatalf("%#v", req.Clauses[6])
	}
}
