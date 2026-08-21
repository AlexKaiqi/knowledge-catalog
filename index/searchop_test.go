package index_test

import (
	"testing"

	"kc/index"
	"kc/local"
	"kc/internal/testkit"
	"kc/kernel"
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
				"db":   map[string]any{"type": "string", "access": []any{"filter", "key"}},
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
	if err != nil || len(in) != 2 {
		t.Fatalf("IN: %d %v", len(in), err)
	}
	neq, err := idx.Search(repo, reader.SearchOf(reader.SearchEXISTS("db"), reader.SearchNEQ("db", "tl")))
	if err != nil || len(neq) != 1 || string(neq[0].Address.ObjectID) != "Table:b" {
		t.Fatalf("NEQ: %#v %v", objectIDs(neq), err)
	}
	ex, err := idx.Search(repo, reader.SearchOf(reader.SearchEXISTS("db")))
	if err != nil || len(ex) != 3 {
		t.Fatalf("EXISTS: %d %v", len(ex), err)
	}
	gt, err := idx.Search(repo, reader.SearchOf(reader.SearchRange(reader.OpGT, "n", "5")))
	if err != nil || len(gt) != 1 || string(gt[0].Address.ObjectID) != "Table:b" {
		t.Fatalf("GT: %#v %v", objectIDs(gt), err)
	}
	pathMatch, err := idx.Search(repo, reader.SearchOf(reader.SearchClause{Op: reader.OpMatch, Path: "note", Value: "events"}))
	if err != nil || len(pathMatch) != 2 {
		t.Fatalf("MATCH path: %d %v", len(pathMatch), err)
	}
	sorted, err := idx.Search(repo, reader.SearchOf(reader.SearchEXISTS("db"), reader.SearchSORT("when", "asc")))
	if err != nil || len(sorted) != 3 {
		t.Fatalf("SORT: %d %v", len(sorted), err)
	}
	got := objectIDs(sorted)
	if got[0] != "Table:b" || got[1] != "Table:a" || got[2] != "Table:c" {
		t.Fatalf("SORT order %v", got)
	}
	if kernel.CodeOf(mustSearchErr(t, idx, repo, reader.SearchOf(reader.SearchEQ("note", "x")))) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("EQ on text path")
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

func objectIDs(hits []repository.KnowledgeValue) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = string(h.Address.ObjectID)
	}
	return out
}
