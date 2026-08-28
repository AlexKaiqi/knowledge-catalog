package index_test

import (
	"testing"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
	"kc/snapshot"
)

type batchCountingRepository struct {
	knowledge.Repository
	batchCalls int
	readCalls  int
	batches    [][]knowledge.ObjectID
}

type headPoisonRepository struct {
	*batchCountingRepository
	headCalls int
}

func (r *headPoisonRepository) Head(string) (kernel.CommitID, error) {
	r.headCalls++
	return "", kernel.Fail(kernel.ErrPreconditionFailed, "consumer attempted to follow HEAD")
}

func (r *batchCountingRepository) Read(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	r.readCalls++
	return r.Repository.Read(objectID, commit)
}

func (r *batchCountingRepository) ReadMany(objectIDs []knowledge.ObjectID, commit kernel.CommitID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	r.batchCalls++
	r.batches = append(r.batches, append([]knowledge.ObjectID(nil), objectIDs...))
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

func (r *batchCountingRepository) SchemaObjectIDs(commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	return r.Repository.(knowledge.SchemaStore).SchemaObjectIDs(commit)
}

func TestSearchHydratesCandidatePageThroughKnowledgeBatchReader(t *testing.T) {
	repo := testkit.MakeRepository(t, uniqueIndexRepositoryID("kr://acme/public/core"))
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
	idx := liveIndex(t)
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
	candidateBatches := 0
	for _, ids := range wrapped.batches {
		if len(ids) == 2 && ids[0] == "policy/a" && ids[1] == "policy/b" {
			candidateBatches++
		}
	}
	if candidateBatches != 1 || wrapped.readCalls != 0 {
		t.Fatalf("SEARCH must batch hydrate exactly the current candidate page: batches=%v read=%d", wrapped.batches, wrapped.readCalls)
	}
}

func TestSearchRejectsWrongCandidateCoordinatesBeforeAuthorityHydrate(t *testing.T) {
	repo := testkit.MakeRepository(t, uniqueIndexRepositoryID("kr://acme/public/poison-search"))
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy.body"}, Value: map[string]any{
				"entity": "Policy", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}},
			}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/a"},
				Value: map[string]any{"body": "runbook"}, SchemaRef: "schema/policy.body"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := &staleCandidateEngine{candidates: []index.CandidateRef{{
		Repository: repo.ID(), ObjectID: "policy/a", Basis: "wrong-commit",
	}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, commit); err != nil {
		t.Fatal(err)
	}
	poison := &batchCountingRepository{Repository: repo}
	_, err = idx.SearchAt(poison, commit, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("wrong-basis candidate must fail closed: %v", err)
	}
	for _, batch := range poison.batches {
		for _, objectID := range batch {
			if objectID == "policy/a" {
				t.Fatalf("invalid candidate must not be hydrated from authority: batches=%v", poison.batches)
			}
		}
	}
	if poison.readCalls != 0 {
		t.Fatalf("invalid candidate must not trigger point reads: %d", poison.readCalls)
	}
}

func TestSearchRequiresExplicitFixedCommit(t *testing.T) {
	repo := testkit.MakeRepository(t, uniqueIndexRepositoryID("kr://acme/public/explicit-search-basis"))
	idx := index.NewIndexEngine("", nil)
	_, err := idx.SearchAt(repo, "", retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("empty search basis must be rejected before provider access: %v", err)
	}
}

func TestSearchAtNeverFollowsHeadAfterBasisIsFixed(t *testing.T) {
	repo := testkit.MakeRepository(t, uniqueIndexRepositoryID("kr://acme/public/frozen-search"))
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy.body"}, Value: map[string]any{
				"entity": "Policy", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}},
			}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/a"},
				Value: map[string]any{"body": "runbook"}, SchemaRef: "schema/policy.body"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := &supersetEngine{ids: []knowledge.ObjectID{"policy/a"}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, commit); err != nil {
		t.Fatal(err)
	}
	poison := &headPoisonRepository{batchCountingRepository: &batchCountingRepository{Repository: repo}}
	result, err := idx.SearchAt(poison, commit, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(result.Hits) != 1 {
		t.Fatalf("fixed-basis search: %#v %v", result, err)
	}
	if poison.headCalls != 0 {
		t.Fatalf("fixed-basis search followed HEAD %d times", poison.headCalls)
	}
}
