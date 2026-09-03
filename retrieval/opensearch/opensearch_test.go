package opensearch_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
	"kc/retrieval/opensearch"
	"kc/snapshot"
)

func TestOpenSearchOperators(t *testing.T) {
	idx := liveOpenSearch(t)
	repo := testkit.MakeRepository(t, uniqueESRepo(t, "ops"))
	root := testkit.MustHead(t, repo, "refs/heads/main")
	head := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/dw.table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "pattern": "record",
			"fields": map[string]any{
				"db":       map[string]any{"type": "string", "access": []any{"filter"}},
				"note":     map[string]any{"access": []any{"text"}},
				"n":        map[string]any{"type": "number", "access": []any{"filter"}},
				"optional": map[string]any{"type": "string", "access": []any{"filter"}},
				"when":     map[string]any{"type": "string", "access": []any{"sort"}},
			},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:a", AspectName: "structure"}, Value: map[string]any{"db": "tl", "note": "user events", "n": 2, "when": "2024-01-02"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:b", AspectName: "structure"}, Value: map[string]any{"db": "dw", "note": "billing events", "n": 10, "when": "2024-01-01"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:c", AspectName: "structure"}, Value: map[string]any{"db": "tl", "note": "other", "n": 5, "when": "2024-01-03"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "relation:owned"}, Value: map[string]any{
			"relationId": "relation:owned", "relationType": "owned-by", "direction": "DIRECTED",
			"endpoints": []any{
				map[string]any{"role": "subject", "objectRef": map[string]any{"repository": string(repo.ID()), "object": "Table:a"}},
				map[string]any{"role": "owner", "objectRef": map[string]any{"repository": string(repo.ID()), "object": "Team:finance"}},
			},
		}},
	})
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	registry := snapshot.NewRegistry()
	if err := registry.Add(repo); err != nil {
		t.Fatal(err)
	}
	servingRepo, err := reader.NewReader(registry).Require(repo.ID(), kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := servingRepo.(knowledge.BatchReadStore); !ok {
		t.Fatal("Knowledge service must expose batch Canonical hydration")
	}

	match, err := idx.SearchAt(servingRepo, head, retrieval.SearchOf(retrieval.SearchMATCH("events")))
	if err != nil || len(match.Hits) != 2 {
		t.Fatalf("MATCH: %d %v", len(match.Hits), err)
	}
	if match.Completeness != retrieval.CompletenessComplete {
		t.Fatalf("OpenSearch MATCH modes are implemented exactly: %#v", match)
	}
	eq, err := idx.SearchAt(servingRepo, head, retrieval.SearchOf(retrieval.SearchEQ("db", "dw")))
	if err != nil || len(eq.Hits) != 1 || string(eq.Hits[0].Knowledge.Address.ObjectID) != "Table:b" {
		t.Fatalf("EQ: %#v %v", objectIDs(eq), err)
	}
	in, err := idx.SearchAt(servingRepo, head, retrieval.SearchOf(retrieval.SearchIN("db", "tl", "xx")))
	if err != nil || len(in.Hits) != 2 {
		t.Fatalf("IN: %d %v", len(in.Hits), err)
	}
	ex, err := idx.SearchAt(servingRepo, head, retrieval.SearchOf(retrieval.SearchEXISTS("db")))
	if err != nil || len(ex.Hits) != 3 {
		t.Fatalf("EXISTS: %d %v", len(ex.Hits), err)
	}
	missing, err := idx.SearchAt(servingRepo, head, retrieval.SearchOf(retrieval.SearchMISSING("optional")))
	if err != nil || len(missing.Hits) != 3 {
		t.Fatalf("MISSING: %d %v", len(missing.Hits), err)
	}
	prefix, err := idx.SearchAt(servingRepo, head, retrieval.SearchOf(retrieval.SearchPREFIX("db", "t")))
	if err != nil || len(prefix.Hits) != 2 {
		t.Fatalf("PREFIX: %d %v", len(prefix.Hits), err)
	}
	contains, err := idx.SearchAt(servingRepo, head, retrieval.SearchOf(retrieval.SearchCONTAINS("db", "l")))
	if err != nil || len(contains.Hits) != 2 {
		t.Fatalf("CONTAINS substring: %d %v", len(contains.Hits), err)
	}
	containsOne, err := idx.SearchAt(servingRepo, head, retrieval.SearchOf(retrieval.SearchCONTAINS("db", "w")))
	if err != nil || len(containsOne.Hits) != 1 || containsOne.Hits[0].Knowledge.Address.ObjectID != "Table:b" {
		t.Fatalf("CONTAINS distinguished from PREFIX: %#v %v", objectIDs(containsOne), err)
	}
	phrase, err := idx.SearchAt(servingRepo, head, retrieval.SearchOf(retrieval.SearchMATCHMode("billing events", retrieval.MatchPhrase)))
	if err != nil || len(phrase.Hits) != 1 {
		t.Fatalf("Phrase: %d %v", len(phrase.Hits), err)
	}
	pageRequest := retrieval.SearchOf(retrieval.SearchMATCH("events"))
	pageRequest.Limit = 1
	firstPage, err := idx.SearchAt(servingRepo, head, pageRequest)
	if err != nil || len(firstPage.Hits) != 1 || firstPage.Continuation == "" {
		t.Fatalf("PIT first page: %#v %v", objectIDs(firstPage), err)
	}
	pageRequest.Continuation = firstPage.Continuation
	secondPage, err := idx.SearchAt(servingRepo, head, pageRequest)
	if err != nil || len(secondPage.Hits) != 1 || secondPage.Continuation != "" || secondPage.Hits[0].Knowledge.Address.ObjectID == firstPage.Hits[0].Knowledge.Address.ObjectID {
		t.Fatalf("PIT second page: first=%#v second=%#v err=%v", objectIDs(firstPage), objectIDs(secondPage), err)
	}
	ranged, err := idx.SearchAt(servingRepo, head, retrieval.SearchOf(retrieval.SearchRange(retrieval.OpGT, "n", "5")))
	if err != nil || len(ranged.Hits) != 1 || ranged.Hits[0].Knowledge.Address.ObjectID != "Table:b" {
		t.Fatalf("typed GT: %#v %v", objectIDs(ranged), err)
	}
	neq, err := idx.SearchAt(servingRepo, head, retrieval.SearchOf(retrieval.SearchEXISTS("db"), retrieval.SearchNEQ("db", "tl")))
	if err != nil || len(neq.Hits) != 1 || neq.Hits[0].Knowledge.Address.ObjectID != "Table:b" {
		t.Fatalf("NEQ: %#v %v", objectIDs(neq), err)
	}
	sorted, err := idx.SearchAt(servingRepo, head, retrieval.SearchOf(retrieval.SearchEXISTS("db"), retrieval.SearchSORT("when", "desc")))
	if err != nil || fmt.Sprint(objectIDs(sorted)) != "[Table:c Table:a Table:b]" {
		t.Fatalf("SORT desc: %#v %v", objectIDs(sorted), err)
	}
	relations, err := idx.RelationsAt(servingRepo, head, retrieval.RelationPageRequest{Query: retrieval.RelationQuery{
		Endpoint: knowledge.KnowledgeRef{Repository: servingRepo.ID(), Object: "Table:a"}, RelationType: "owned-by", Role: "subject",
	}})
	if err != nil || len(relations.Hits) != 1 || relations.Hits[0].ObjectID != "relation:owned" || len(relations.Hits[0].MatchedRoles) != 1 {
		t.Fatalf("relation lookup: %#v %v", relations, err)
	}
}

func TestOpenSearchIncrementalAndSchemaRebuild(t *testing.T) {
	idx := liveOpenSearch(t)
	repo := testkit.MakeRepository(t, uniqueESRepo(t, "incr"))
	root := testkit.MustHead(t, repo, "refs/heads/main")
	c1 := putAt(t, repo, root, []knowledge.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "needs a runbook"}, "")[0],
	})
	first, err := idx.Apply(repo, root, c1, []knowledge.ObjectID{"policy/P-1"})
	if err != nil || first.Mode != index.IndexModeRebuild {
		t.Fatalf("%#v %v", first, err)
	}
	c2 := putAt(t, repo, c1, testkit.PutEntity("policy/P-2", map[string]any{"body": "another runbook"}, ""))
	second, err := idx.Apply(repo, c1, c2, []knowledge.ObjectID{"policy/P-2"})
	if err != nil || second.Mode != index.IndexModeIncremental || second.Cause != index.IndexCauseContent || second.Updated != 1 {
		t.Fatalf("want content incremental, got %#v %v", second, err)
	}
	hits, err := idx.SearchAt(repo, c2, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(hits.Hits) != 2 {
		t.Fatalf("after incremental %d %v", len(hits.Hits), err)
	}
	c3 := putAt(t, repo, c2, []knowledge.Operation{{
		Op: knowledge.OpRemove, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/P-2"},
	}})
	removed, err := idx.Apply(repo, c2, c3, []knowledge.ObjectID{"policy/P-2"})
	if err != nil || removed.Mode != index.IndexModeIncremental || removed.Removed != 1 {
		t.Fatalf("remove %#v %v", removed, err)
	}
	hits, err = idx.SearchAt(repo, c3, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(hits.Hits) != 1 {
		t.Fatalf("after remove %d %v", len(hits.Hits), err)
	}

	c4 := putAt(t, repo, c3, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy.body"}, Value: map[string]any{
			"entity": "Policy", "pattern": "record",
			"fields": map[string]any{
				"body": map[string]any{"access": []any{"text"}},
				"tag":  map[string]any{"access": []any{"filter"}},
			},
		}},
	})
	rebuilt, err := idx.Apply(repo, c3, c4, []knowledge.ObjectID{"schema/policy.body"})
	if err != nil || rebuilt.Mode != index.IndexModeRebuild || rebuilt.Cause != index.IndexCauseSchema {
		t.Fatalf("schema rebuild %#v %v", rebuilt, err)
	}
}

func liveOpenSearch(t *testing.T) *index.Index {
	t.Helper()
	if testing.Short() && strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")) == "" {
		t.Skip("short OpenSearch tests require testsuite.sh to provide KC_TEST_OPENSEARCH_URL")
	}
	idx := index.NewIndexEngine("", opensearch.Open(opensearch.Config{URL: os.Getenv("KC_TEST_OPENSEARCH_URL"), PrimaryShards: 1}))
	t.Cleanup(func() { _ = idx.Close() })
	probe := testkit.MakeRepository(t, uniqueESRepo(t, "ping"))
	if _, err := idx.Ensure(probe, testkit.MustHead(t, probe, "refs/heads/main")); err != nil {
		t.Skip(err)
	}
	return idx
}

func uniqueESRepo(t *testing.T, kind string) string {
	t.Helper()
	return fmt.Sprintf("kr://conformance/es/%s/%d", kind, time.Now().UnixNano())
}
