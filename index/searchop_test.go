package index_test

import (
	"strings"
	"testing"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/local"
	"kc/reader"
	"kc/repository"
)

func TestSearchAtomicOperators(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/physical")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	head := putAt(t, repo, root, []repository.Operation{
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/dw.table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "pattern": "record",
			"fields": map[string]any{
				"db":   map[string]any{"type": "string", "access": []any{"filter"}},
				"note": map[string]any{"access": []any{"text"}},
				"n":    map[string]any{"type": "number", "access": []any{"filter"}},
				"when": map[string]any{"type": "string", "access": []any{"sort"}},
			},
		}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Table:a", AspectName: "structure"}, Value: map[string]any{"db": "tl", "note": "user events", "n": 2, "when": "2024-01-02"}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Table:b", AspectName: "structure"}, Value: map[string]any{"db": "dw", "note": "billing events", "n": 10, "when": "2024-01-01"}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Table:c", AspectName: "structure"}, Value: map[string]any{"db": "tl", "note": "other", "n": 5, "when": "2024-01-03"}},
	})
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}

	in, err := idx.Search(repo, reader.SearchOf(reader.SearchIN("db", "tl", "xx")))
	if err != nil || len(in.Hits) != 2 {
		t.Fatalf("IN: %d %v", len(in.Hits), err)
	}
	neq, err := idx.Search(repo, reader.SearchOf(reader.SearchEXISTS("db"), reader.SearchNEQ("db", "tl")))
	if err != nil || len(neq.Hits) != 1 || string(neq.Hits[0].Knowledge.Address.ObjectID) != "Table:b" {
		t.Fatalf("NEQ: %#v %v", objectIDs(neq), err)
	}
	ex, err := idx.Search(repo, reader.SearchOf(reader.SearchEXISTS("db")))
	if err != nil || len(ex.Hits) != 3 {
		t.Fatalf("EXISTS: %d %v", len(ex.Hits), err)
	}
	gt, err := idx.Search(repo, reader.SearchOf(reader.SearchRange(reader.OpGT, "n", "5")))
	if err != nil || len(gt.Hits) != 1 || string(gt.Hits[0].Knowledge.Address.ObjectID) != "Table:b" {
		t.Fatalf("GT: %#v %v", objectIDs(gt), err)
	}
	pathMatch, err := idx.Search(repo, reader.SearchOf(reader.SearchClause{Op: reader.OpMatch, Path: "note", Value: "events"}))
	if err != nil || len(pathMatch.Hits) != 2 {
		t.Fatalf("MATCH path: %d %v", len(pathMatch.Hits), err)
	}
	sorted, err := idx.Search(repo, reader.SearchOf(reader.SearchEXISTS("db"), reader.SearchSORT("when", "asc")))
	if err != nil || len(sorted.Hits) != 3 {
		t.Fatalf("SORT: %d %v", len(sorted.Hits), err)
	}
	got := objectIDs(sorted)
	if got[0] != "Table:b" || got[1] != "Table:a" || got[2] != "Table:c" {
		t.Fatalf("SORT order %v", got)
	}
	if kernel.CodeOf(mustSearchErr(t, idx, repo, reader.SearchOf(reader.SearchEQ("note", "x")))) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("EQ on text path")
	}
}

func TestSearchCommonMVPAndPublicContinuation(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/search-mvp")
	root := testkit.MustHead(t, repo, repository.DefaultRef)
	head := putAt(t, repo, root, []repository.Operation{
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/item.structure"}, Value: map[string]any{
			"entity": "Item", "aspect": "structure", "pattern": "record",
			"fields": map[string]any{
				"name": map[string]any{"type": "string", "access": []any{"filter"}},
				"note": map[string]any{"type": "string", "access": []any{"text"}},
				"n":    map[string]any{"type": "number", "access": []any{"filter"}},
				"day":  map[string]any{"type": "date", "access": []any{"filter"}},
				"tags": map[string]any{"type": "string", "access": []any{"filter"}},
			},
		}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Item:a", AspectName: "structure"}, Value: map[string]any{"name": "customer.orders", "note": "billing daily events", "n": 2, "day": "2024-01-02"}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Item:b", AspectName: "structure"}, Value: map[string]any{"name": "customer.items", "note": "billing archive", "n": 10, "day": "2024-02-01"}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Item:c", AspectName: "structure"}, Value: map[string]any{"name": "staging.orders", "note": "daily only", "n": 5, "day": "2023-12-31", "tags": []any{"blue", "gold"}}},
	})
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	assertHits := func(label string, request reader.SearchRequest, want ...string) reader.SearchResult {
		t.Helper()
		result, err := idx.Search(repo, request)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		got := objectIDs(result)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s: got %v want %v", label, got, want)
		}
		return result
	}
	assertHits("all terms", reader.SearchOf(reader.SearchMATCHMode("billing events", reader.MatchAllTerms)), "Item:a")
	assertHits("any terms", reader.SearchOf(reader.SearchMATCHMode("billing events", reader.MatchAnyTerms)), "Item:a", "Item:b")
	assertHits("phrase", reader.SearchOf(reader.SearchMATCHMode("daily events", reader.MatchPhrase)), "Item:a")
	assertHits("missing", reader.SearchOf(reader.SearchMISSING("tags")), "Item:a", "Item:b")
	assertHits("prefix", reader.SearchOf(reader.SearchPREFIX("name", "customer.")), "Item:a", "Item:b")
	assertHits("typed number", reader.SearchOf(reader.SearchRange(reader.OpGT, "n", "5")), "Item:b")
	assertHits("typed date", reader.SearchOf(reader.SearchRange(reader.OpLT, "day", "2024-01-01")), "Item:c")
	assertHits("neq excludes missing", reader.SearchOf(reader.SearchNEQ("tags", "blue")))
	if _, err := idx.Search(repo, reader.SearchOf(reader.SearchRange(reader.OpGT, "n", "five"))); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("invalid typed scalar: %v", err)
	}
	firstReq := reader.SearchOf(reader.SearchPREFIX("name", "customer."))
	firstReq.Limit = 1
	first := assertHits("first page", firstReq, "Item:a")
	if first.Continuation == "" {
		t.Fatal("first page must expose an opaque continuation")
	}
	secondReq := reader.SearchOf(reader.SearchPREFIX("name", "customer."))
	secondReq.Limit = 1
	secondReq.Continuation = first.Continuation
	second := assertHits("second page", secondReq, "Item:b")
	if second.Continuation != "" {
		t.Fatalf("last page continuation: %q", second.Continuation)
	}
	wrong := reader.SearchOf(reader.SearchPREFIX("name", "staging."))
	wrong.Limit = 1
	wrong.Continuation = first.Continuation
	if _, err := idx.Search(repo, wrong); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("cross-query continuation: %v", err)
	}
	next := putAt(t, repo, head, []repository.Operation{{
		Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Item:d", AspectName: "structure"},
		Value: map[string]any{"name": "customer.new", "note": "new", "n": 1, "day": "2024-03-01"},
	}})
	if _, err := idx.Apply(repo, head, next, []kernel.ObjectID{"Item:d"}); err != nil {
		t.Fatal(err)
	}
	oldView := reader.SearchOf(reader.SearchPREFIX("name", "customer."))
	oldView.Limit = 1
	oldView.Continuation = first.Continuation
	if _, err := idx.Search(repo, oldView); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("cross-view continuation: %v", err)
	}
}

func mustSearchErr(t *testing.T, idx *index.Index, repo repository.Repository, req reader.SearchRequest) error {
	t.Helper()
	_, err := idx.Search(repo, req)
	if err == nil {
		t.Fatal("expected error")
	}
	return err
}

func objectIDs(hits reader.SearchResult) []string {
	out := make([]string, len(hits.Hits))
	for i, h := range hits.Hits {
		out[i] = string(h.Knowledge.Address.ObjectID)
	}
	return out
}
