package opensearch_test

import (
	"fmt"
	"kc/declarativeindex/internal/testfixture"
	"math"
	"testing"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestOpenSearchExactTemporalAndIntegerPagination(t *testing.T) {
	idx := liveOpenSearch(t)
	repo := testkit.MakeRepository(t, uniqueESRepo(t, "precision"))
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	ops := []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/event.precision"}, Value: map[string]any{
		"entity": "Event", "aspect": "precision", "pattern": "record", "fields": map[string]any{
			"when": map[string]any{"type": "timestamp"},
			"n":    map[string]any{"type": "integer"},
		},
	}},
		testkit.IndexField("schema/event.precision", "when", "date", "filter", "sort"),
		testkit.IndexField("schema/event.precision", "n", "number", "filter", "sort"),
	}
	for _, item := range []struct {
		id, when string
		n        int64
	}{
		{"event/z", "0000-01-01T00:00:00Z", math.MinInt64},
		{"event/y", "2026-01-01T00:00:00.000000001Z", 9007199254740992},
		{"event/a", "2026-01-01T00:00:00.000000002Z", 9007199254740993},
		{"event/b", "9999-12-31T23:59:59.999999999Z", math.MaxInt64},
	} {
		ops = append(ops, objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: objectmodel.ObjectID(item.id), AspectName: "precision"}, Value: map[string]any{"when": item.when, "n": item.n}})
	}
	head := testfixture.PutAt(t, repo, root, ops)
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
						got = append(got, string(hit.Object.Address.ObjectID))
					}
					if result.Continuation == "" {
						break
					}
					req.Continuation = result.Continuation
				}
				want := "[event/z event/y event/a event/b]"
				if order == "desc" {
					want = "[event/b event/a event/y event/z]"
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
		{"nanosecond EQ", []retrieval.SearchClause{retrieval.SearchEQ("when", "2026-01-01T00:00:00.000000001Z")}, "[event/y]"},
		{"timezone equivalent", []retrieval.SearchClause{retrieval.SearchEQ("when", "2026-01-01T08:00:00.000000001+08:00")}, "[event/y]"},
		{"nanosecond range", []retrieval.SearchClause{retrieval.SearchRange(retrieval.OpGT, "when", "2026-01-01T00:00:00.000000001Z"), retrieval.SearchRange(retrieval.OpLT, "when", "2026-01-02T00:00:00Z")}, "[event/a]"},
		{"integer EQ", []retrieval.SearchClause{retrieval.SearchEQ("n", "9007199254740993")}, "[event/a]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := idx.SearchAt(repo, head, retrieval.SearchOf(tc.clauses...))
			if err != nil || fmt.Sprint(objectIDs(result)) != tc.want {
				t.Fatalf("got=%v want=%s err=%v", objectIDs(result), tc.want, err)
			}
		})
	}
}
