package projection_test

import (
	"kc/declarativeindex/internal/testfixture"
	ports "kc/discovery/projection"
	"testing"

	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

type batchCountingRepository struct {
	objectmodel.Repository
	batchCalls int
	readCalls  int
	batches    [][]objectmodel.ObjectID
}

type headPoisonRepository struct {
	*batchCountingRepository
	headCalls int
}

func (r *headPoisonRepository) Head(string) (kernel.CommitID, error) {
	r.headCalls++
	return "", kernel.Fail(kernel.ErrPreconditionFailed, "consumer attempted to follow HEAD")
}

func (r *batchCountingRepository) Read(objectID objectmodel.ObjectID, commit kernel.CommitID) (objectmodel.KnowledgeValue, error) {
	r.readCalls++
	return r.Repository.Read(objectID, commit)
}

func (r *batchCountingRepository) ReadMany(objectIDs []objectmodel.ObjectID, commit kernel.CommitID) (map[objectmodel.ObjectID]objectmodel.KnowledgeValue, error) {
	r.batchCalls++
	r.batches = append(r.batches, append([]objectmodel.ObjectID(nil), objectIDs...))
	out := map[objectmodel.ObjectID]objectmodel.KnowledgeValue{}
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

func (r *batchCountingRepository) SchemaObjectIDs(commit kernel.CommitID) ([]objectmodel.ObjectID, error) {
	return r.Repository.(objectmodel.SchemaStore).SchemaObjectIDs(commit)
}

func TestSearchHydratesCandidatePageThroughKnowledgeBatchReader(t *testing.T) {
	repo := testfixture.MakeIndexRepository(t, "kr://acme/public/core")
	root, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.ApplyKnowledgeCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy.body"}, Value: map[string]any{
				"entity": "Policy", "pattern": "record",
				"fields": map[string]any{"body": map[string]any{"access": []any{"text"}}},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/a"}, Value: map[string]any{"body": "runbook alpha"}, SchemaRef: "schema/policy.body"},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/b"}, Value: map[string]any{"body": "runbook beta"}, SchemaRef: "schema/policy.body"},
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
	repo := testfixture.MakeIndexRepository(t, "kr://acme/public/poison-search")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyKnowledgeCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy.body"}, Value: map[string]any{
				"entity": "Policy", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/a"},
				Value: map[string]any{"body": "runbook"}, SchemaRef: "schema/policy.body"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := &staleCandidateEngine{candidates: []ports.CandidateRef{{
		Repository: repo.ID(), ObjectID: "policy/a", Basis: "wrong-commit",
	}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
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
	repo := testfixture.MakeIndexRepository(t, "kr://acme/public/explicit-search-basis")
	idx := index.NewIndexEngine("", nil)
	_, err := idx.SearchAt(repo, "", retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("empty search basis must be rejected before provider access: %v", err)
	}
}

func TestSearchAtNeverFollowsHeadAfterBasisIsFixed(t *testing.T) {
	repo := testfixture.MakeIndexRepository(t, "kr://acme/public/frozen-search")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyKnowledgeCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy.body"}, Value: map[string]any{
				"entity": "Policy", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/a"},
				Value: map[string]any{"body": "runbook"}, SchemaRef: "schema/policy.body"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := &staleCandidateEngine{candidates: []ports.CandidateRef{{ObjectID: "policy/a", Basis: commit}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
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
