package index_test

import (
	"fmt"
	"strconv"
	"testing"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
	"kc/snapshot"
)

type relationEngine struct {
	meta  index.Meta
	pages [][]retrieval.RelationCandidate
	calls *[]string
}

func (e *relationEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) index.Capability {
	return index.Capability{Guarantee: index.GuaranteeUnsupported}
}
func (e *relationEngine) Retrieve(index.RetrieveRequest) (index.CandidatePage, error) {
	return index.CandidatePage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "search is not configured")
}
func (e *relationEngine) RetrieveRelations(request retrieval.RelationRetrieveRequest) (retrieval.RelationCandidatePage, error) {
	*e.calls = append(*e.calls, "retrieve")
	pageIndex := 0
	if request.Continuation != "" {
		var err error
		pageIndex, err = strconv.Atoi(request.Continuation)
		if err != nil {
			return retrieval.RelationCandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "invalid continuation")
		}
	}
	if pageIndex >= len(e.pages) {
		return retrieval.RelationCandidatePage{Exhausted: true}, nil
	}
	page := retrieval.RelationCandidatePage{Candidates: e.pages[pageIndex], Exhausted: pageIndex == len(e.pages)-1}
	if !page.Exhausted {
		page.Continuation = strconv.Itoa(pageIndex + 1)
	}
	return page, nil
}
func (e *relationEngine) LoadMeta() (index.Meta, error)                 { return e.meta, nil }
func (e *relationEngine) Rebuild([]index.CompiledDoc, index.Meta) error { return nil }
func (e *relationEngine) Apply([]index.CompiledDoc, []knowledge.ObjectID, index.Meta) error {
	return nil
}
func (e *relationEngine) Count() (int, error) { return 0, nil }
func (e *relationEngine) Close() error        { return nil }

type noRelationEngine struct{ relationEngine }

type poisonAuthority struct {
	knowledge.Repository
	batch   knowledge.BatchReadStore
	allowed map[knowledge.ObjectID]bool
	calls   *[]string
	missing map[knowledge.ObjectID]bool
}

func (r *poisonAuthority) Read(knowledge.ObjectID, kernel.CommitID) (knowledge.KnowledgeValue, error) {
	*r.calls = append(*r.calls, "forbidden-read")
	return knowledge.KnowledgeValue{}, fmt.Errorf("poison authority: point Read is forbidden")
}

func (r *poisonAuthority) ReadMany(ids []knowledge.ObjectID, commit kernel.CommitID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	*r.calls = append(*r.calls, "read-many")
	for _, id := range ids {
		if !r.allowed[id] {
			return nil, fmt.Errorf("poison authority: unexpected candidate %s", id)
		}
	}
	values, err := r.batch.ReadMany(ids, commit)
	if err != nil {
		return nil, err
	}
	for id := range r.missing {
		delete(values, id)
	}
	return values, nil
}

func relationFixture(t *testing.T) (*testkit.KnowledgeRepository, kernel.CommitID) {
	t.Helper()
	repo := testkit.MakeRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "relation:owned"}, Value: relationBody(repo.ID(), "relation:owned", "Table:a")},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "relation:false-positive"}, Value: relationBody(repo.ID(), "relation:false-positive", "Table:other")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo, commit
}

func relationBody(repository kernel.RepositoryID, id, endpoint string) map[string]any {
	return map[string]any{
		"relationId": id, "relationType": "owned-by", "direction": "DIRECTED",
		"endpoints": []any{
			map[string]any{"role": "subject", "objectRef": map[string]any{"repository": string(repository), "object": endpoint}},
			map[string]any{"role": "owner", "objectRef": map[string]any{"repository": string(repository), "object": "Team:finance"}},
		},
	}
}

func relationRequest(repository kernel.RepositoryID) retrieval.RelationPageRequest {
	return retrieval.RelationPageRequest{Query: retrieval.RelationQuery{
		Endpoint: knowledge.KnowledgeRef{Repository: repository, Object: "Table:a"}, RelationType: "owned-by", Role: "subject",
	}}
}

func relationBatch(t *testing.T, repo *testkit.KnowledgeRepository) knowledge.BatchReadStore {
	t.Helper()
	batch, ok := repo.Repository.(knowledge.BatchReadStore)
	if !ok {
		t.Fatal("fixture repository must expose provider-neutral ReadMany")
	}
	return batch
}

func TestRelationsUsesExactRetrieverBeforePoisonAuthorityReadMany(t *testing.T) {
	repo, commit := relationFixture(t)
	calls := []string{}
	engine := &relationEngine{meta: index.Meta{Basis: commit, State: index.ProjectionStateReady, Generation: "g1"}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{{{Repository: repo.ID(), ObjectID: "relation:owned", Basis: commit}}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	poison := &poisonAuthority{Repository: repo, batch: relationBatch(t, repo), allowed: map[knowledge.ObjectID]bool{"relation:owned": true}, calls: &calls}
	page, err := idx.RelationsAt(poison, commit, relationRequest(repo.ID()))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].Relation.RelationID != "relation:owned" {
		t.Fatalf("canonical oracle mismatch: %#v", page)
	}
	if fmt.Sprint(calls) != "[retrieve read-many]" {
		t.Fatalf("want Retrieve -> ReadMany only, got %v", calls)
	}
}

func TestRelationsRejectsWrongBasisBeforeAuthority(t *testing.T) {
	repo, commit := relationFixture(t)
	calls := []string{}
	engine := &relationEngine{meta: index.Meta{Basis: commit, State: index.ProjectionStateReady}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{{{Repository: repo.ID(), ObjectID: "relation:owned", Basis: "wrong"}}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	poison := &poisonAuthority{Repository: repo, batch: relationBatch(t, repo), allowed: map[knowledge.ObjectID]bool{}, calls: &calls}
	_, err := idx.RelationsAt(poison, commit, relationRequest(repo.ID()))
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
	if fmt.Sprint(calls) != "[retrieve]" {
		t.Fatalf("authority must remain untouched: %v", calls)
	}
}

func TestRelationsRequiresReadyProviderBeforeAuthority(t *testing.T) {
	repo, commit := relationFixture(t)
	for _, tc := range []struct {
		name  string
		state string
		code  kernel.ErrorCode
	}{{"missing", "", kernel.ErrCapabilityUnsatisfied}, {"building", index.ProjectionStateBuilding, kernel.ErrTemporaryUnavailable}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			engine := &relationEngine{meta: index.Meta{Basis: commit, State: tc.state}, calls: &calls}
			idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
			poison := &poisonAuthority{Repository: repo, batch: relationBatch(t, repo), allowed: map[knowledge.ObjectID]bool{}, calls: &calls}
			_, err := idx.RelationsAt(poison, commit, relationRequest(repo.ID()))
			testkit.ExpectCode(t, err, tc.code)
			if len(calls) != 0 {
				t.Fatalf("authority/retriever must remain untouched: %v", calls)
			}
		})
	}
}

func TestRelationsPagesCandidatesAndRechecksFalsePositives(t *testing.T) {
	repo, commit := relationFixture(t)
	calls := []string{}
	engine := &relationEngine{meta: index.Meta{Basis: commit, State: index.ProjectionStateReady}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{
			{{Repository: repo.ID(), ObjectID: "relation:false-positive", Basis: commit}},
			{{Repository: repo.ID(), ObjectID: "relation:owned", Basis: commit}},
		}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	poison := &poisonAuthority{Repository: repo, batch: relationBatch(t, repo), allowed: map[knowledge.ObjectID]bool{
		"relation:false-positive": true, "relation:owned": true,
	}, calls: &calls}
	request := relationRequest(repo.ID())
	request.Limit = 1
	page, err := idx.RelationsAt(poison, commit, request)
	if err != nil || len(page.Hits) != 1 || page.Hits[0].ObjectID != "relation:owned" {
		t.Fatalf("false-positive residual: %#v %v", page, err)
	}
	if len(page.Claims) != 1 {
		t.Fatalf("false-positive must produce projection consistency evidence: %#v", page.Claims)
	}
	if fmt.Sprint(calls) != "[retrieve read-many retrieve read-many]" {
		t.Fatalf("candidate pages must hydrate independently: %v", calls)
	}
}

func TestRelationsTreatsMissingCandidateAsProjectionConsistencyError(t *testing.T) {
	repo, commit := relationFixture(t)
	calls := []string{}
	engine := &relationEngine{meta: index.Meta{Basis: commit, State: index.ProjectionStateReady}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{{{Repository: repo.ID(), ObjectID: "relation:owned", Basis: commit}}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	poison := &poisonAuthority{Repository: repo, batch: relationBatch(t, repo), allowed: map[knowledge.ObjectID]bool{"relation:owned": true},
		missing: map[knowledge.ObjectID]bool{"relation:owned": true}, calls: &calls}
	_, err := idx.RelationsAt(poison, commit, relationRequest(repo.ID()))
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
}

func TestRelationsContinuationBindsQueryBasisAndGeneration(t *testing.T) {
	repo, commit := relationFixture(t)
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	if root != commit {
		t.Fatalf("fixture head mismatch: %s != %s", root, commit)
	}
	next, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: commit, ExpectedTargetCommit: commit,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "relation:owned-2"},
			Value: relationBody(repo.ID(), "relation:owned-2", "Table:a"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	engine := &relationEngine{meta: index.Meta{Basis: next, State: index.ProjectionStateReady, Generation: "g1"}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{
			{{Repository: repo.ID(), ObjectID: "relation:owned", Basis: next}},
			{{Repository: repo.ID(), ObjectID: "relation:owned-2", Basis: next}},
		}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	request := relationRequest(repo.ID())
	request.Limit = 1
	first, err := idx.RelationsAt(repo, next, request)
	if err != nil || len(first.Hits) != 1 || first.Continuation == "" || first.Exhausted {
		t.Fatalf("first page: %#v %v", first, err)
	}
	request.Continuation = first.Continuation
	second, err := idx.RelationsAt(repo, next, request)
	if err != nil || len(second.Hits) != 1 || second.Continuation != "" || !second.Exhausted {
		t.Fatalf("second page: %#v %v", second, err)
	}

	position := len(first.Continuation) / 2
	replacement := "A"
	if first.Continuation[position] == 'A' {
		replacement = "B"
	}
	tampered := first.Continuation[:position] + replacement + first.Continuation[position+1:]
	request.Continuation = tampered
	_, err = idx.RelationsAt(repo, next, request)
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)

	request.Continuation = first.Continuation
	request.Query.Role = "owner"
	_, err = idx.RelationsAt(repo, next, request)
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)

	engine.meta.Generation = "g2"
	request.Query.Role = "subject"
	_, err = idx.RelationsAt(repo, next, request)
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
}
