package index_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/knowledge"
	"kc/retrieval"
	"kc/snapshot"
)

// Exercise the published knowledge path, not a manually assembled AccessSpec.
func TestPublishedSchemaTypedSearchPreservesLongTextIntegersAndTime(t *testing.T) {
	repo := makeIndexRepository(t, "kr://mvp/typed")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	ops := []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{
		"entity": "Article", "fields": map[string]any{
			"body": map[string]any{"type": "string", "access": []any{"text"}},
			"rank": map[string]any{"type": "integer", "access": []any{"filter", "sort"}},
			"when": map[string]any{"type": "timestamp", "access": []any{"filter", "sort"}},
			"day":  map[string]any{"type": "date", "access": []any{"filter", "sort"}},
		},
	}}}
	for n, stamp := range []string{"2026-09-08T00:00:00.000000001Z", "2026-09-08T08:00:00.000000002+08:00", "2026-09-08T00:00:00.000000003Z"} {
		ops = append(ops, knowledge.Operation{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: knowledge.ObjectID(fmt.Sprintf("article/%d", n))}, SchemaRef: "schema/article", Value: map[string]any{
			"body": strings.Repeat("规范知识 reliability ", 4000), "rank": int64(9007199254740992) + int64(n), "when": stamp, "day": "2026-09-08",
		}})
	}
	head := putAt(t, repo, root, ops)
	idx := liveIndex(t)
	if _, err := idx.Ensure(repo, head); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		query retrieval.SearchRequest
		want  []string
	}{
		{"integer-eq", retrieval.SearchOf(retrieval.SearchEQ("rank", "9007199254740993")), []string{"article/1"}},
		{"integer-range", retrieval.SearchOf(retrieval.SearchRange(retrieval.OpGT, "rank", "9007199254740992"), retrieval.SearchSORT("rank", "asc")), []string{"article/1", "article/2"}},
		{"time-eq", retrieval.SearchOf(retrieval.SearchEQ("when", "2026-09-08T00:00:00.000000002Z")), []string{"article/1"}},
		{"time-range", retrieval.SearchOf(retrieval.SearchRange(retrieval.OpGT, "when", "2026-09-08T00:00:00.000000001Z"), retrieval.SearchSORT("when", "asc")), []string{"article/1", "article/2"}},
		{"date-eq", retrieval.SearchOf(retrieval.SearchEQ("day", "2026-09-08")), []string{"article/0", "article/1", "article/2"}},
		{"long-text", retrieval.SearchOf(retrieval.SearchMATCH("reliability")), []string{"article/0", "article/1", "article/2"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := idx.SearchAt(repo, head, test.query)
			if err != nil {
				t.Fatal(err)
			}
			if got.Completeness != retrieval.CompletenessComplete || !reflect.DeepEqual(mvpHitIDs(got), test.want) {
				t.Fatalf("got=%v completeness=%s want=%v", mvpHitIDs(got), got.Completeness, test.want)
			}
			for _, hit := range got.Hits {
				if hit.Knowledge.Commit != head || hit.Version.DeclarationCommit != head {
					t.Fatalf("lost fixed basis: %+v", hit.Version)
				}
			}
		})
	}
	for _, field := range []string{"rank", "when"} {
		request := retrieval.SearchOf(retrieval.SearchEXISTS(field), retrieval.SearchSORT(field, "asc"))
		request.Limit = 1
		var ids []string
		for page := 0; page < 4; page++ {
			result, err := idx.SearchAt(repo, head, request)
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, mvpHitIDs(result)...)
			request.Continuation = result.Continuation
			if request.Continuation == "" {
				break
			}
		}
		if !reflect.DeepEqual(ids, []string{"article/0", "article/1", "article/2"}) || request.Continuation != "" {
			t.Fatalf("%s pagination lost or repeated an identity: %v", field, ids)
		}
	}
	value, err := repo.Read("article/1", head)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(value.Value)
	if err != nil || !strings.Contains(string(raw), "9007199254740993") {
		t.Fatalf("authority changed integer before indexing: %s %v", raw, err)
	}
}

// This validates the available consumption operations with explicit decisions;
// it makes no claim about a language model's mapping or exploration quality.
func TestControlledVocabularyAndProgressiveReadingUseFixedKnowledgeBasis(t *testing.T) {
	repo := makeIndexRepository(t, "kr://mvp/exploration")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/concept"}, Value: map[string]any{"entity": "Concept", "fields": map[string]any{
			"alias": map[string]any{"type": "string", "access": []any{"filter", "text"}},
		}}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/guide"}, Value: map[string]any{"entity": "Guide", "fields": map[string]any{
			"concept": map[string]any{"type": "object_ref", "access": []any{"filter"}},
		}}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "concept/refund"}, SchemaRef: "schema/concept", Value: map[string]any{"alias": "退货退款", "definition": "退回已支付的金额"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "guide/refund"}, SchemaRef: "schema/guide", Value: map[string]any{"concept": "concept/refund", "body": "退款条件见政策；例外情况需要进一步阅读。"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "guide/exception"}, Value: map[string]any{"body": "退款例外需提供原始凭据。"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "relation/refund-exception"}, Value: map[string]any{
			"relationId": "relation/refund-exception", "relationType": "explained-by", "direction": "DIRECTED", "endpoints": []any{
				map[string]any{"role": "subject", "objectRef": map[string]any{"repository": string(repo.ID()), "object": "guide/refund"}},
				map[string]any{"role": "detail", "objectRef": map[string]any{"repository": string(repo.ID()), "object": "guide/exception"}},
			},
		}},
	})
	idx := liveIndex(t)
	if _, err := idx.Ensure(repo, head); err != nil {
		t.Fatal(err)
	}
	concepts, err := idx.SearchAt(repo, head, retrieval.SearchOf(retrieval.SearchEQ("alias", "退货退款")))
	if err != nil || !reflect.DeepEqual(mvpHitIDs(concepts), []string{"concept/refund"}) {
		t.Fatalf("vocabulary lookup: %v %v", mvpHitIDs(concepts), err)
	}
	conceptRef := concepts.Hits[0].Knowledge.Address.ObjectID
	guides, err := idx.SearchAt(repo, head, retrieval.SearchOf(retrieval.SearchEQ("concept", string(conceptRef))))
	if err != nil || !reflect.DeepEqual(mvpHitIDs(guides), []string{"guide/refund"}) {
		t.Fatalf("mapped concept lookup: %v %v", mvpHitIDs(guides), err)
	}
	relations, err := idx.RelationsAt(repo, head, retrieval.RelationPageRequest{Query: retrieval.RelationQuery{Endpoint: knowledge.KnowledgeRef{Repository: repo.ID(), Object: "guide/refund"}, RelationType: "explained-by", Role: "subject"}})
	if err != nil || len(relations.Hits) != 1 {
		t.Fatalf("progressive relation lookup: %+v %v", relations, err)
	}
	var detail knowledge.KnowledgeRef
	for _, endpoint := range relations.Hits[0].Relation.Endpoints {
		if endpoint.Role == "detail" {
			detail = endpoint.ObjectRef
		}
	}
	if detail.Object != "guide/exception" || detail.Repository != repo.ID() {
		t.Fatalf("lost evidence identity: %+v", detail)
	}
	// Advance HEAD after locating the next material. Reading the fixed basis
	// must still return the evidence used by this exploration.
	putAt(t, repo, head, []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: detail.Object}, Value: map[string]any{"body": "新版本说明"}}})
	value, err := repo.Read(detail.Object, head)
	if err != nil || value.Commit != head || value.Value.(map[string]any)["body"] != "退款例外需提供原始凭据。" {
		t.Fatalf("progressive read mixed versions: %+v %v", value, err)
	}
}

func mvpHitIDs(result retrieval.SearchResult) []string {
	ids := make([]string, len(result.Hits))
	for i, hit := range result.Hits {
		ids[i] = string(hit.Knowledge.Address.ObjectID)
	}
	return ids
}
