package index_test

import (
	"testing"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
	"kc/retrieval/sqlite"
	"kc/snapshot"
	"kc/snapshot/filegit"
)

type batchCountingRepository struct {
	knowledge.Repository
	batchCalls int
	readCalls  int
}

func (r *batchCountingRepository) Read(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	r.readCalls++
	return r.Repository.Read(objectID, commit)
}

func (r *batchCountingRepository) ReadMany(objectIDs []knowledge.ObjectID, commit kernel.CommitID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	r.batchCalls++
	out := map[knowledge.ObjectID]knowledge.KnowledgeValue{}
	for _, objectID := range objectIDs {
		value, err := r.Repository.Read(objectID, commit)
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[objectID] = value
	}
	return out, nil
}

func TestSearchHydratesCandidatePageThroughKnowledgeBatchReader(t *testing.T) {
	raw, err := filegit.NewFileGit(t.TempDir(), "kr://acme/public/core")
	if err != nil {
		t.Fatal(err)
	}
	repo := testkit.OpenRepository(t, raw)
	root, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy.body"}, Value: map[string]any{
				"entity": "Policy", "pattern": "record",
				"fields": map[string]any{"body": map[string]any{"access": []any{"text"}}},
			}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/a"}, Value: map[string]any{"body": "runbook alpha"}, SchemaRef: "schema/policy.body"},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/b"}, Value: map[string]any{"body": "runbook beta"}, SchemaRef: "schema/policy.body"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	idx := index.NewIndexEngine("", sqlite.Open)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Ensure(repo, commit); err != nil {
		t.Fatal(err)
	}
	wrapped := &batchCountingRepository{Repository: repo}
	result, err := idx.SearchAt(wrapped, commit, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 2 {
		t.Fatalf("unexpected hits: %#v", result.Hits)
	}
	if wrapped.batchCalls != 1 || wrapped.readCalls != 0 {
		t.Fatalf("SEARCH must batch hydrate one candidate page: batch=%d read=%d", wrapped.batchCalls, wrapped.readCalls)
	}
}
