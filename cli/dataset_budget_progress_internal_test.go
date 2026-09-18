package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"kc/catalog"
	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
	"kc/snapshot"
)

type workspaceBudgetEngine struct {
	meta     index.Meta
	docs     []index.CompiledDoc
	superset bool
	requests []int
}

func (e *workspaceBudgetEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) index.Capability {
	guarantee := index.GuaranteeExact
	if e.superset {
		guarantee = index.GuaranteeSuperset
	}
	return index.Capability{Guarantee: guarantee, Coverage: 1}
}
func (e *workspaceBudgetEngine) Retrieve(req index.RetrieveRequest) (index.CandidatePage, error) {
	e.requests = append(e.requests, req.Search.Limit)
	offset, _ := strconv.Atoi(req.Continuation)
	end := offset + req.Search.Limit
	if end > len(e.docs) {
		end = len(e.docs)
	}
	out := index.CandidatePage{Exhausted: end == len(e.docs)}
	for _, doc := range e.docs[offset:end] {
		out.Candidates = append(out.Candidates, index.CandidateRef{ObjectID: doc.ObjectID, Basis: e.meta.Basis})
	}
	if !out.Exhausted {
		out.Continuation = strconv.Itoa(end)
	}
	return out, nil
}
func (e *workspaceBudgetEngine) LoadMeta() (index.Meta, error) { return e.meta, nil }
func (e *workspaceBudgetEngine) Rebuild(docs []index.CompiledDoc, meta index.Meta) error {
	e.docs = append([]index.CompiledDoc(nil), docs...)
	sort.Slice(e.docs, func(i, j int) bool { return e.docs[i].ObjectID < e.docs[j].ObjectID })
	e.meta = meta
	return nil
}
func (*workspaceBudgetEngine) Apply([]index.CompiledDoc, []knowledge.ObjectID, index.Meta) error {
	return nil
}
func (e *workspaceBudgetEngine) Count() (int, error) { return len(e.docs), nil }
func (*workspaceBudgetEngine) Close() error          { return nil }

func workspaceBudgetFixture(t *testing.T, members, objects int, superset bool) (map[kernel.RepositoryID]*workspaceBudgetEngine, func(index.SearchBudget, string) (retrieval.SearchResult, error)) {
	t.Helper()
	registry := snapshot.NewRegistry()
	engines := map[kernel.RepositoryID]*workspaceBudgetEngine{}
	idx := index.NewIndexEngine("", func(_ string, id kernel.RepositoryID) (index.Engine, error) {
		engine := engines[id]
		if engine == nil {
			return nil, fmt.Errorf("unexpected engine %s", id)
		}
		return engine, nil
	})
	t.Cleanup(func() { _ = idx.Close() })
	sources := []catalog.KnowledgeSetSource{}
	for member := 0; member < members; member++ {
		id := kernel.RepositoryID(fmt.Sprintf("kr://acme/public/member-%03d", member))
		repo := testkit.MakeRepository(t, string(id))
		if err := registry.Add(repo); err != nil {
			t.Fatal(err)
		}
		head := testkit.MustHead(t, repo, snapshot.DefaultRef)
		ops := []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/item"}, Value: map[string]any{"entity": "Item", "fields": map[string]any{"status": map[string]any{"type": "string", "access": []any{"filter"}}}}}}
		for n := 0; n < objects; n++ {
			status := "yes"
			if superset && n < objects-1 {
				status = "no"
			}
			ops = append(ops, knowledge.Operation{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: knowledge.ObjectID(fmt.Sprintf("item/%03d", n))}, SchemaRef: "schema/item", Value: map[string]any{"status": status}})
		}
		commit, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{TargetRepository: id, TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head, Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		engines[id] = &workspaceBudgetEngine{superset: superset}
		if _, err := idx.Rebuild(repo, commit); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, catalog.KnowledgeSetSource{Repository: id, Selector: snapshot.DefaultRef})
	}
	cat, err := catalog.NewCatalog(registry, testkit.MakeRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	repositories := make([]string, len(sources))
	for i, source := range sources {
		repositories[i] = string(source.Repository)
	}
	// Seed registry state once: this test exercises consumption, not 101 separate
	// registry publications. Every knowledge body above still crosses Writer.
	cat.LoadState(catalog.CatalogState{CatalogID: "kr://acme/catalog", Repositories: repositories, KnowledgeSets: []catalog.KnowledgeSet{{SetID: "budget", Revision: 1, Sources: sources}}})
	dir := t.TempDir()
	ws := &Home{Dir: dir, Store: registry, Reader: reader.NewReader(registry), Index: idx, Catalog: cat, Catalogs: map[string]*catalog.Catalog{"kr://acme/catalog": cat}, File: HomeFile{Catalogs: []HomeCatalog{{ID: "kr://acme/catalog"}}}}
	cx := &invocation{Home: dir, WS: ws, Flags: map[string]FlagValue{"dataset": "budget", "as": "agent:budget"}}
	run := func(budget index.SearchBudget, token string) (retrieval.SearchResult, error) {
		ctx, cancel := index.WithSearchBudget(context.Background(), budget)
		defer cancel()
		cx.Context = ctx
		req := retrieval.SearchOf(retrieval.SearchEQ("status", "yes"))
		req.Limit = 1
		req.Continuation = token
		cx.Flags["_search-request"] = req
		value, err := searchWorkspace(cx)
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		return value.(retrieval.SearchResult), nil
	}
	return engines, run
}

func TestWorkspaceBudgetCannotReturnEndlessEmptyContinuation(t *testing.T) {
	_, run := workspaceBudgetFixture(t, 2, 1, false)
	budget := index.SearchBudget{MaxCandidates: 1, MaxPages: 100, Timeout: time.Second}
	token := ""
	for attempt := 0; attempt < 3; attempt++ {
		out, err := run(budget, token)
		if kernel.CodeOf(err) == kernel.ErrTemporaryUnavailable {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("attempt=%d hits=%d partial=%s tokenEqual=%v", attempt, len(out.Hits), out.Completeness, token == out.Continuation)
		if len(out.Hits) > 0 || out.Continuation == "" {
			t.Fatalf("insufficient head budget should fail explicitly: %#v", out)
		}
		token = out.Continuation
	}
	t.Fatal("three requests consumed budget without producing a hit or explicit failure")
}

func TestWorkspaceBudgetFairHeadsProduceProgress(t *testing.T) {
	_, run := workspaceBudgetFixture(t, 2, 4, false)
	out, err := run(index.SearchBudget{MaxCandidates: 2, MaxPages: 100, Timeout: time.Second}, "")
	if err != nil || len(out.Hits) != 1 {
		t.Fatalf("two candidates can establish two heads: hits=%d completeness=%s err=%v", len(out.Hits), out.Completeness, err)
	}
}

func TestWorkspaceBudgetResidualEmptyPageRetainsRealProgress(t *testing.T) {
	_, run := workspaceBudgetFixture(t, 1, 3, true)
	budget := index.SearchBudget{MaxCandidates: 1, MaxPages: 100, Timeout: time.Second}
	token := ""
	for attempt := 0; attempt < 3; attempt++ {
		out, err := run(budget, token)
		if err != nil {
			t.Fatal(err)
		}
		if attempt < 2 {
			if len(out.Hits) != 0 || out.Continuation == "" {
				t.Fatalf("residual page %d: %#v", attempt, out)
			}
			outer, err := retrieval.DecodeContinuation(out.Continuation)
			if err != nil {
				t.Fatal(err)
			}
			member, err := retrieval.DecodeContinuation(outer.Members[0].Position)
			if err != nil {
				t.Fatal(err)
			}
			if member.Position != strconv.Itoa(attempt+1) {
				t.Fatalf("real provider position did not advance: %#v", member)
			}
		} else if len(out.Hits) != 1 {
			t.Fatalf("residual paging never reached matching object: %#v", out)
		}
		token = out.Continuation
	}
}

func TestWorkspaceBudgetDefaultPageCapCannotStall101Members(t *testing.T) {
	_, run := workspaceBudgetFixture(t, 101, 1, false)
	out, err := run(index.SearchBudget{Timeout: 5 * time.Second}, "")
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("default cap must fail explicitly: hits=%d continuation=%s err=%v", len(out.Hits), strings.Repeat("x", min(len(out.Continuation), 10)), err)
	}
}

func TestWorkspaceBudgetOffsetReplayDebtCanAdvanceOnEmptyPages(t *testing.T) {
	_, run := workspaceBudgetFixture(t, 1, 6, false)
	seed, err := run(index.SearchBudget{MaxCandidates: 100, Timeout: time.Second}, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := retrieval.DecodeContinuation(seed.Continuation)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a valid cursor issued by the prior fixed 16-hit batch: five hits
	// from its first batch have already been delivered, but no body is in token.
	state.Members[0].Position = ""
	state.Members[0].Offset = 5
	state.Members[0].Exhausted = false
	token := retrieval.EncodeContinuation(state)
	budget := index.SearchBudget{MaxCandidates: 1, MaxPages: 100, Timeout: time.Second}
	for debt := 5; debt > 0; debt-- {
		page, err := run(budget, token)
		if err != nil {
			t.Fatal(err)
		}
		next, err := retrieval.DecodeContinuation(page.Continuation)
		if err != nil {
			t.Fatal(err)
		}
		member, err := retrieval.DecodeContinuation(next.Members[0].Position)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Hits) != 0 || next.Members[0].Offset != debt-1 || member.Position != strconv.Itoa(6-debt) {
			t.Fatalf("replay debt did not advance: hits=%d member=%#v provider=%#v", len(page.Hits), next.Members[0], member)
		}
		token = page.Continuation
	}
	last, err := run(budget, token)
	if err != nil || len(last.Hits) != 1 || last.Hits[0].Knowledge.Address.ObjectID != "item/005" {
		t.Fatalf("replay lost next unread hit: %#v %v", last, err)
	}
}

func TestWorkspaceBudgetPrimingReplaysSavedOffsetInOneBatch(t *testing.T) {
	engines, run := workspaceBudgetFixture(t, 1, 6, false)
	budget := index.SearchBudget{MaxCandidates: 100, Timeout: time.Second}
	seed, err := run(budget, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := retrieval.DecodeContinuation(seed.Continuation)
	if err != nil {
		t.Fatal(err)
	}
	state.Members[0].Position = ""
	state.Members[0].Offset = 5
	state.Members[0].Exhausted = false
	engine := engines[state.Members[0].Repository]
	engine.requests = nil
	out, err := run(budget, retrieval.EncodeContinuation(state))
	if err != nil || len(out.Hits) != 1 || out.Hits[0].Knowledge.Address.ObjectID != "item/005" {
		t.Fatalf("resumed unread hit: %#v %v", out, err)
	}
	if len(engine.requests) != 1 || engine.requests[0] != 6 {
		t.Fatalf("offset five should need one six-candidate fetch; actual requests=%v", engine.requests)
	}
}
