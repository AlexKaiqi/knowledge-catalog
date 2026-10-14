package projection_test

import (
	"fmt"
	"kc/declarativeindex/internal/testfixture"
	ports "kc/discovery/projection"
	"strconv"
	"testing"

	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

type relationEngine struct {
	meta  ports.Meta
	pages [][]retrieval.RelationCandidate
	calls *[]string
}

func (e *relationEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) ports.Capability {
	return ports.Capability{Guarantee: ports.GuaranteeUnsupported}
}
func (e *relationEngine) Retrieve(ports.RetrieveRequest) (ports.CandidatePage, error) {
	return ports.CandidatePage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "search is not configured")
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
func (e *relationEngine) LoadMeta() (ports.Meta, error)                 { return e.meta, nil }
func (e *relationEngine) Rebuild([]ports.CompiledDoc, ports.Meta) error { return nil }
func (e *relationEngine) Apply([]ports.CompiledDoc, []objectmodel.ObjectID, ports.Meta) error {
	return nil
}
func (e *relationEngine) Count() (int, error) { return 0, nil }
func (e *relationEngine) Close() error        { return nil }

type poisonAuthority struct {
	objectmodel.Repository
	batch   objectmodel.BatchReadStore
	allowed map[objectmodel.ObjectID]bool
	calls   *[]string
	missing map[objectmodel.ObjectID]bool
}

func (r *poisonAuthority) Read(objectmodel.ObjectID, kernel.CommitID) (objectmodel.KnowledgeValue, error) {
	*r.calls = append(*r.calls, "forbidden-read")
	return objectmodel.KnowledgeValue{}, fmt.Errorf("poison authority: point Read is forbidden")
}

func (r *poisonAuthority) ReadMany(ids []objectmodel.ObjectID, commit kernel.CommitID) (map[objectmodel.ObjectID]objectmodel.KnowledgeValue, error) {
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

func relationRequest(repository kernel.RepositoryID) retrieval.RelationPageRequest {
	return retrieval.RelationPageRequest{Query: retrieval.RelationQuery{
		Endpoint: objectmodel.KnowledgeRef{Repository: repository, Object: "Table:a"}, RelationType: "owned-by", Role: "subject",
	}}
}

func relationBatch(t *testing.T, repo *testkit.KnowledgeRepository) objectmodel.BatchReadStore {
	t.Helper()
	batch, ok := repo.Repository.(objectmodel.BatchReadStore)
	if !ok {
		t.Fatal("fixture repository must expose provider-neutral ReadMany")
	}
	return batch
}

func TestRelationsUsesExactRetrieverBeforePoisonAuthorityReadMany(t *testing.T) {
	repo, commit := testfixture.RelationFixture(t)
	calls := []string{}
	engine := &relationEngine{meta: ports.Meta{Basis: commit, State: ports.ProjectionStateReady, Generation: "g1"}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{{{Repository: repo.ID(), ObjectID: "relation:owned", Basis: commit}}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	poison := &poisonAuthority{Repository: repo, batch: relationBatch(t, repo), allowed: map[objectmodel.ObjectID]bool{"relation:owned": true}, calls: &calls}
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
	repo, commit := testfixture.RelationFixture(t)
	calls := []string{}
	engine := &relationEngine{meta: ports.Meta{Basis: commit, State: ports.ProjectionStateReady}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{{{Repository: repo.ID(), ObjectID: "relation:owned", Basis: "wrong"}}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	poison := &poisonAuthority{Repository: repo, batch: relationBatch(t, repo), allowed: map[objectmodel.ObjectID]bool{}, calls: &calls}
	_, err := idx.RelationsAt(poison, commit, relationRequest(repo.ID()))
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
	if fmt.Sprint(calls) != "[retrieve]" {
		t.Fatalf("authority must remain untouched: %v", calls)
	}
}

func TestRelationsRequiresReadyProviderBeforeAuthority(t *testing.T) {
	repo, commit := testfixture.RelationFixture(t)
	for _, tc := range []struct {
		name  string
		state string
		code  kernel.ErrorCode
	}{{"missing", "", kernel.ErrCapabilityUnsatisfied}, {"building", ports.ProjectionStateBuilding, kernel.ErrTemporaryUnavailable}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			engine := &relationEngine{meta: ports.Meta{Basis: commit, State: tc.state}, calls: &calls}
			idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
			poison := &poisonAuthority{Repository: repo, batch: relationBatch(t, repo), allowed: map[objectmodel.ObjectID]bool{}, calls: &calls}
			_, err := idx.RelationsAt(poison, commit, relationRequest(repo.ID()))
			testkit.ExpectCode(t, err, tc.code)
			if len(calls) != 0 {
				t.Fatalf("authority/retriever must remain untouched: %v", calls)
			}
		})
	}
}

func TestRelationsPagesCandidatesAndRechecksFalsePositives(t *testing.T) {
	repo, commit := testfixture.RelationFixture(t)
	calls := []string{}
	engine := &relationEngine{meta: ports.Meta{Basis: commit, State: ports.ProjectionStateReady}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{
			{{Repository: repo.ID(), ObjectID: "relation:false-positive", Basis: commit}},
			{{Repository: repo.ID(), ObjectID: "relation:owned", Basis: commit}},
		}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	poison := &poisonAuthority{Repository: repo, batch: relationBatch(t, repo), allowed: map[objectmodel.ObjectID]bool{
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

func TestRelationsRejectsInvalidLimit(t *testing.T) {
	repo, commit := testfixture.RelationFixture(t)
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) {
		t.Fatal("invalid page limit must fail before opening a provider")
		return nil, nil
	})
	request := relationRequest(repo.ID())
	request.Limit = 1001
	_, err := idx.RelationsAt(repo, commit, request)
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

func TestRelationsTreatsMissingCandidateAsProjectionConsistencyError(t *testing.T) {
	repo, commit := testfixture.RelationFixture(t)
	calls := []string{}
	engine := &relationEngine{meta: ports.Meta{Basis: commit, State: ports.ProjectionStateReady}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{{{Repository: repo.ID(), ObjectID: "relation:owned", Basis: commit}}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	poison := &poisonAuthority{Repository: repo, batch: relationBatch(t, repo), allowed: map[objectmodel.ObjectID]bool{"relation:owned": true},
		missing: map[objectmodel.ObjectID]bool{"relation:owned": true}, calls: &calls}
	_, err := idx.RelationsAt(poison, commit, relationRequest(repo.ID()))
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
}

func TestRelationsContinuationBindsQueryBasisAndGeneration(t *testing.T) {
	repo, commit := testfixture.RelationFixture(t)
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	if root != commit {
		t.Fatalf("fixture head mismatch: %s != %s", root, commit)
	}
	next, err := repo.ApplyKnowledgeCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: commit, ExpectedTargetCommit: commit,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "relation:owned-2"},
			Value: testfixture.RelationBody(repo.ID(), "relation:owned-2", "Table:a"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	engine := &relationEngine{meta: ports.Meta{Basis: next, State: ports.ProjectionStateReady, Generation: "g1"}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{
			{{Repository: repo.ID(), ObjectID: "relation:owned", Basis: next}},
			{{Repository: repo.ID(), ObjectID: "relation:owned-2", Basis: next}},
		}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
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
