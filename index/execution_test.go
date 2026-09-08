package index_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
	"kc/snapshot"
)

func TestSearchBudgetStopsResidualPagesAndCanResume(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/budget")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/item"}, Value: map[string]any{
			"entity": "Item", "fields": map[string]any{"status": map[string]any{"type": "string", "access": []any{"filter"}}},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "item/a"}, Value: map[string]any{"status": "no"}, SchemaRef: "schema/item"},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "item/b"}, Value: map[string]any{"status": "no"}, SchemaRef: "schema/item"},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "item/c"}, Value: map[string]any{"status": "yes"}, SchemaRef: "schema/item"},
	})
	engine := &supersetEngine{ids: []knowledge.ObjectID{"item/a", "item/b", "item/c"}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	request := retrieval.SearchOf(retrieval.SearchEQ("status", "yes"))
	request.Limit = 1
	ctx, cancel := index.WithSearchBudget(context.Background(), index.SearchBudget{MaxCandidates: 2, MaxPages: 2, Timeout: time.Second})
	defer cancel()
	first, err := idx.SearchAtContext(ctx, repo, head, request)
	if err != nil || first.Completeness != retrieval.CompletenessPartial || first.Continuation == "" || len(first.Hits) != 0 || first.Stats.Candidates != 2 {
		t.Fatalf("budget result: %#v %v", first, err)
	}
	request.Continuation = first.Continuation
	second, err := idx.SearchAt(repo, head, request)
	if err != nil || len(second.Hits) != 1 || second.Hits[0].Knowledge.Address.ObjectID != "item/c" || second.Completeness != retrieval.CompletenessComplete {
		t.Fatalf("resumed result: %#v %v", second, err)
	}
}

type cancellableEngine struct {
	staleCandidateEngine
	calls int
}

func (e *cancellableEngine) RetrieveContext(ctx context.Context, _ index.RetrieveRequest) (index.CandidatePage, error) {
	e.calls++
	<-ctx.Done()
	return index.CandidatePage{}, ctx.Err()
}

func TestSearchPropagatesCancellationToContextProvider(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/cancel-search")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := putAt(t, repo, root, []knowledge.Operation{policyBodySchema()})
	engine := &cancellableEngine{}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(10*time.Millisecond, cancel)
	_, err := idx.SearchAtContext(ctx, repo, head, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if !errors.Is(err, context.Canceled) || engine.calls != 1 {
		t.Fatalf("cancellation: calls=%d err=%v", engine.calls, err)
	}
}

func TestSearchTimeBudgetReturnsResumablePartial(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/deadline-search")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := putAt(t, repo, root, []knowledge.Operation{policyBodySchema()})
	engine := &cancellableEngine{}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := index.WithSearchBudget(context.Background(), index.SearchBudget{Timeout: 10 * time.Millisecond})
	defer cancel()
	got, err := idx.SearchAtContext(ctx, repo, head, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || got.Completeness != retrieval.CompletenessPartial || got.Continuation == "" || engine.calls != 1 {
		t.Fatalf("time budget: %#v calls=%d err=%v", got, engine.calls, err)
	}
}

type unverifiedExpressionEngine struct{ *supersetEngine }

func (*unverifiedExpressionEngine) ProbeExpression(retrieval.SearchExpr, retrieval.AccessSpec) index.Capability {
	return index.Capability{Guarantee: index.GuaranteeSuperset, Coverage: 1}
}

func TestSupersetMatchWithoutAnalysisProofFailsClosed(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/residual-analysis")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := putAt(t, repo, root, []knowledge.Operation{policyBodySchema()})
	engine := &unverifiedExpressionEngine{&supersetEngine{}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	request := retrieval.SearchWhere(retrieval.SearchAny(
		retrieval.SearchLeaf(retrieval.SearchMATCHMode("hello world", retrieval.MatchPhrase)),
		retrieval.SearchLeaf(retrieval.SearchMATCH("cat")),
	))
	if _, err := idx.SearchAt(repo, head, request); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || !strings.Contains(err.Error(), "analyzed MATCH") {
		t.Fatalf("unproved analyzed residual must fail: %v", err)
	}
}

type cancellableMetaEngine struct {
	staleCandidateEngine
	calls int
}

func (e *cancellableMetaEngine) LoadMetaContext(ctx context.Context) (index.Meta, error) {
	e.calls++
	<-ctx.Done()
	return index.Meta{}, ctx.Err()
}
func TestSearchCancellationIncludesMetadataRead(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/cancel-meta")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit := putAt(t, repo, root, []knowledge.Operation{policyBodySchema()})
	engine := &cancellableMetaEngine{}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	defer idx.Close()
	if _, err := idx.Rebuild(repo, commit); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(10*time.Millisecond, cancel)
	_, err := idx.SearchAtContext(ctx, repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if !errors.Is(err, context.Canceled) || engine.calls != 1 {
		t.Fatalf("metadata cancellation: calls=%d err=%v", engine.calls, err)
	}
}

type cancellableRelationEngine struct {
	relationEngine
	calls int
}

func (e *cancellableRelationEngine) RetrieveRelationsContext(ctx context.Context, _ retrieval.RelationRetrieveRequest) (retrieval.RelationCandidatePage, error) {
	e.calls++
	<-ctx.Done()
	return retrieval.RelationCandidatePage{}, ctx.Err()
}
func TestRelationTimeBudgetReturnsResumableBoundary(t *testing.T) {
	repo, commit := relationFixture(t)
	engine := &cancellableRelationEngine{relationEngine: relationEngine{meta: index.Meta{Basis: commit, State: index.ProjectionStateReady, Generation: "g1"}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	defer idx.Close()
	ctx, cancel := index.WithSearchBudget(context.Background(), index.SearchBudget{Timeout: 10 * time.Millisecond})
	defer cancel()
	got, err := idx.RelationsAtContext(ctx, repo, commit, relationRequest(repo.ID()))
	if err != nil || got.Exhausted || got.Continuation == "" || len(got.Claims) == 0 || engine.calls != 1 {
		t.Fatalf("relation budget: %#v calls=%d %v", got, engine.calls, err)
	}
}

func TestSearchPreparationTimeoutIsTemporaryUnavailable(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/timeout-meta")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit := putAt(t, repo, root, []knowledge.Operation{policyBodySchema()})
	engine := &cancellableMetaEngine{}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	defer idx.Close()
	if _, err := idx.Rebuild(repo, commit); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := index.WithSearchBudget(context.Background(), index.SearchBudget{Timeout: 10 * time.Millisecond})
	defer cancel()
	result, err := idx.SearchAtContext(ctx, repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || result.Continuation != "" {
		t.Fatalf("unplanned timeout invented a continuation: %#v %v", result, err)
	}
}
