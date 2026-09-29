package opensearch_test

import (
	"fmt"
	"math"
	"testing"

	"kc/internal/testkit"
	"kc/knowledge"
	"kc/retrieval"
	"kc/snapshot"
)

func TestOpenSearchExactTemporalAndIntegerPagination(t *testing.T) {
	idx := liveOpenSearch(t)
	repo := testkit.MakeRepository(t, uniqueESRepo(t, "precision"))
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	ops := []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/event.precision"}, Value: map[string]any{
		"entity": "Event", "aspect": "precision", "pattern": "record", "fields": map[string]any{
			"when": map[string]any{"type": "timestamp", "access": []any{"filter", "sort"}},
			"n":    map[string]any{"type": "integer", "access": []any{"filter", "sort"}},
		},
	}}}
	for _, item := range []struct {
		id, when string
		n        int64
	}{
		{"Event:z", "0000-01-01T00:00:00Z", math.MinInt64},
		{"Event:y", "2026-01-01T00:00:00.000000001Z", 9007199254740992},
		{"Event:a", "2026-01-01T00:00:00.000000002Z", 9007199254740993},
		{"Event:b", "9999-12-31T23:59:59.999999999Z", math.MaxInt64},
	} {
		ops = append(ops, knowledge.Operation{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: knowledge.ObjectID(item.id), AspectName: "precision"}, Value: map[string]any{"when": item.when, "n": item.n}})
	}
	head := putAt(t, repo, root, ops)
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"when", "n"} {
		for _, order := range []string{"asc", "desc"} {
			t.Run(field+"/"+order, func(t *testing.T) {
				req := retrieval.SearchOf(retrieval.SearchEXISTS(field), retrieval.SearchSORT(field, order))
				req.Limit = 1
				var got []string
				for page := 0; page < 8; page++ {
					result, err := idx.SearchAt(repo, head, req)
					if err != nil {
						t.Fatal(err)
					}
					for _, hit := range result.Hits {
						got = append(got, string(hit.Knowledge.Address.ObjectID))
					}
					if result.Continuation == "" {
						break
					}
					req.Continuation = result.Continuation
				}
				want := "[Event:z Event:y Event:a Event:b]"
				if order == "desc" {
					want = "[Event:b Event:a Event:y Event:z]"
				}
				if fmt.Sprint(got) != want {
					t.Fatalf("precision-safe pagination: got=%v want=%s", got, want)
				}
			})
		}
	}
	for _, tc := range []struct {
		name    string
		clauses []retrieval.SearchClause
		want    string
	}{
		{"nanosecond EQ", []retrieval.SearchClause{retrieval.SearchEQ("when", "2026-01-01T00:00:00.000000001Z")}, "[Event:y]"},
		{"timezone equivalent", []retrieval.SearchClause{retrieval.SearchEQ("when", "2026-01-01T08:00:00.000000001+08:00")}, "[Event:y]"},
		{"nanosecond range", []retrieval.SearchClause{retrieval.SearchRange(retrieval.OpGT, "when", "2026-01-01T00:00:00.000000001Z"), retrieval.SearchRange(retrieval.OpLT, "when", "2026-01-02T00:00:00Z")}, "[Event:a]"},
		{"integer EQ", []retrieval.SearchClause{retrieval.SearchEQ("n", "9007199254740993")}, "[Event:a]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := idx.SearchAt(repo, head, retrieval.SearchOf(tc.clauses...))
			if err != nil || fmt.Sprint(objectIDs(result)) != tc.want {
				t.Fatalf("got=%v want=%s err=%v", objectIDs(result), tc.want, err)
			}
		})
	}
}
