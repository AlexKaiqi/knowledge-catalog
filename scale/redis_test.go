package scale_test

import (
	"fmt"
	"testing"
	"time"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/reader"
	"kc/repository"
	"kc/scale"
)

func TestRedisConfigRejectsSecrets(t *testing.T) {
	if _, err := scale.ParseRedisAddr("redis://:secret@127.0.0.1:16379"); err == nil {
		t.Fatal("expected redis password to fail")
	}
	cfg, err := scale.ParseRedisAddr("127.0.0.1:16379")
	if err != nil || cfg.Host != "127.0.0.1" || cfg.Port != 16379 {
		t.Fatalf("%#v %v", cfg, err)
	}
	if _, err := scale.OpenRedis(scale.RedisConfig{Password: "nope"})("", "kr://conformance/redis/secret"); err == nil {
		t.Fatal("expected config password to fail")
	}
}

func TestRedisOperators(t *testing.T) {
	idx := liveRedisIndex(t)
	repo := testkit.MakeRepository(t, uniqueRedisRepo(t, "ops"))
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
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}

	match, err := idx.Search(repo, reader.SearchOf(reader.SearchMATCH("events")))
	if err != nil || len(match) != 2 {
		t.Fatalf("MATCH: %d %v", len(match), err)
	}
	eq, err := idx.Search(repo, reader.SearchOf(reader.SearchEQ("db", "dw")))
	if err != nil || len(eq) != 1 || string(eq[0].Address.ObjectID) != "Table:b" {
		t.Fatalf("EQ: %#v %v", objectIDs(eq), err)
	}
	in, err := idx.Search(repo, reader.SearchOf(reader.SearchIN("db", "tl", "xx")))
	if err != nil || len(in) != 2 {
		t.Fatalf("IN: %d %v", len(in), err)
	}
	ex, err := idx.Search(repo, reader.SearchOf(reader.SearchEXISTS("db")))
	if err != nil || len(ex) != 3 {
		t.Fatalf("EXISTS: %d %v", len(ex), err)
	}
	neq, err := idx.Search(repo, reader.SearchOf(reader.SearchEXISTS("db"), reader.SearchNEQ("db", "tl")))
	if err != nil || len(neq) != 1 || string(neq[0].Address.ObjectID) != "Table:b" {
		t.Fatalf("NEQ: %#v %v", objectIDs(neq), err)
	}
	gt, err := idx.Search(repo, reader.SearchOf(reader.SearchRange(reader.OpGT, "n", "5")))
	if err != nil || len(gt) != 1 || string(gt[0].Address.ObjectID) != "Table:b" {
		t.Fatalf("GT: %#v %v", objectIDs(gt), err)
	}
	gte, err := idx.Search(repo, reader.SearchOf(reader.SearchRange(reader.OpGTE, "n", "5")))
	if err != nil || len(gte) != 2 {
		t.Fatalf("GTE: %#v %v", objectIDs(gte), err)
	}
	sorted, err := idx.Search(repo, reader.SearchOf(reader.SearchEXISTS("db"), reader.SearchSORT("when", "asc")))
	if err != nil || len(sorted) != 3 {
		t.Fatalf("SORT: %d %v", len(sorted), err)
	}
	got := objectIDs(sorted)
	if got[0] != "Table:b" || got[1] != "Table:a" || got[2] != "Table:c" {
		t.Fatalf("SORT order %v", got)
	}
}

func TestRedisIncrementalAndSchemaRebuild(t *testing.T) {
	idx := liveRedisIndex(t)
	repo := testkit.MakeRepository(t, uniqueRedisRepo(t, "incr"))
	root := testkit.MustHead(t, repo, "refs/heads/main")
	c1 := putAt(t, repo, root, []repository.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "needs a runbook"}, "")[0],
	})
	first, err := idx.Apply(repo, root, c1, []kernel.ObjectID{"policy/P-1"})
	if err != nil || first.Mode != index.IndexModeRebuild {
		t.Fatalf("%#v %v", first, err)
	}
	c2 := putAt(t, repo, c1, testkit.PutEntity("policy/P-2", map[string]any{"body": "another runbook"}, ""))
	second, err := idx.Apply(repo, c1, c2, []kernel.ObjectID{"policy/P-2"})
	if err != nil || second.Mode != index.IndexModeIncremental || second.Cause != index.IndexCauseContent || second.Updated != 1 {
		t.Fatalf("want content incremental, got %#v %v", second, err)
	}
	hits, err := idx.Search(repo, reader.SearchOf(reader.SearchMATCH("runbook")))
	if err != nil || len(hits) != 2 {
		t.Fatalf("after incremental %d %v", len(hits), err)
	}
	c3 := putAt(t, repo, c2, []repository.Operation{{
		Op: repository.OpRemove, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "policy/P-2"},
	}})
	removed, err := idx.Apply(repo, c2, c3, []kernel.ObjectID{"policy/P-2"})
	if err != nil || removed.Mode != index.IndexModeIncremental || removed.Removed != 1 {
		t.Fatalf("remove %#v %v", removed, err)
	}
	hits, err = idx.Search(repo, reader.SearchOf(reader.SearchMATCH("runbook")))
	if err != nil || len(hits) != 1 {
		t.Fatalf("after remove %d %v", len(hits), err)
	}

	c4 := putAt(t, repo, c3, []repository.Operation{
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/policy.body"}, Value: map[string]any{
			"entity": "Policy", "pattern": "record",
			"fields": map[string]any{
				"body": map[string]any{"access": []any{"text"}},
				"tag":  map[string]any{"access": []any{"filter"}},
			},
		}},
	})
	rebuilt, err := idx.Apply(repo, c3, c4, []kernel.ObjectID{"schema/policy.body"})
	if err != nil || rebuilt.Mode != index.IndexModeRebuild || rebuilt.Cause != index.IndexCauseSchema {
		t.Fatalf("schema rebuild %#v %v", rebuilt, err)
	}
}

func liveRedisIndex(t *testing.T) *index.Index {
	t.Helper()
	idx := index.NewIndexEngine("", scale.OpenRedis(scale.RedisConfig{}))
	t.Cleanup(func() { _ = idx.Close() })
	probe := testkit.MakeRepository(t, uniqueRedisRepo(t, "ping"))
	if _, err := idx.Ensure(probe, testkit.MustHead(t, probe, "refs/heads/main")); err != nil {
		t.Skip(err)
	}
	return idx
}

func uniqueRedisRepo(t *testing.T, kind string) string {
	t.Helper()
	return fmt.Sprintf("kr://conformance/redis/%s/%d", kind, time.Now().UnixNano())
}
