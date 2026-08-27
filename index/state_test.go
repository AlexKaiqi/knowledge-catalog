package index

import (
	"context"
	"errors"
	"sort"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	knowledgeserving "kc/knowledge/serving"
	"kc/retrieval"
	"kc/snapshot"
)

type stateTestEngine struct {
	meta Meta
	docs map[knowledge.ObjectID]CompiledDoc
}

func (e *stateTestEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) Capability {
	return Capability{Guarantee: GuaranteeSuperset, Coverage: 1}
}
func (e *stateTestEngine) Retrieve(RetrieveRequest) (CandidatePage, error) {
	ids := make([]knowledge.ObjectID, 0, len(e.docs))
	for id := range e.docs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	page := CandidatePage{Exhausted: true}
	for _, id := range ids {
		page.Candidates = append(page.Candidates, CandidateRef{ObjectID: id, Basis: e.meta.Basis})
	}
	return page, nil
}
func (e *stateTestEngine) LoadMeta() (Meta, error) { return e.meta, nil }
func (e *stateTestEngine) Rebuild(docs []CompiledDoc, meta Meta) error {
	e.docs = map[knowledge.ObjectID]CompiledDoc{}
	for _, doc := range docs {
		e.docs[doc.ObjectID] = doc
	}
	e.meta = meta
	return nil
}
func (e *stateTestEngine) Apply(upserts []CompiledDoc, deletes []knowledge.ObjectID, meta Meta) error {
	for _, id := range deletes {
		delete(e.docs, id)
	}
	for _, doc := range upserts {
		e.docs[doc.ObjectID] = doc
	}
	e.meta = meta
	return nil
}
func (e *stateTestEngine) Count() (int, error) { return len(e.docs), nil }
func (e *stateTestEngine) Close() error        { return nil }

type stateTestLookup struct {
	value any
	basis knowledge.ObservationBasis
	err   error
}

func (s *stateTestLookup) LookupState(context.Context, knowledgeserving.StateLookupRequest) (knowledgeserving.StateObservation, error) {
	if s.err != nil {
		return knowledgeserving.StateObservation{}, s.err
	}
	return knowledgeserving.StateObservation{Value: s.value, Basis: s.basis}, nil
}

func stateProjectionFixture(t *testing.T) (knowledge.Repository, kernel.CommitID, knowledge.Address) {
	t.Helper()
	setup := testkit.NewSetup(t, "")
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Job:orders", AspectName: "runtime"}
	source := &knowledge.ValueSource{Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{
		Mode: knowledge.BindingState, Runtime: "scheduler", Protocol: "resource-access/v1",
		Operations: map[string]knowledge.BindingOperation{"lookup": {Call: "job.status"}},
	}}
	commit, err := setup.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: setup.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: setup.RootCommitID, ExpectedTargetCommit: setup.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/job.definition"}, Value: map[string]any{
				"entity": "Job", "aspect": "definition", "fields": map[string]any{"owner": map[string]any{"type": "string", "access": []any{"filter"}}},
			}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/job.runtime"}, Value: map[string]any{
				"entity": "Job", "aspect": "runtime", "fields": map[string]any{
					"status": map[string]any{"type": "string", "access": []any{"text", "filter"}},
					"owner":  map[string]any{"type": "string", "access": []any{"filter"}},
				},
			}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: address.ObjectID, AspectName: "definition"}, Value: map[string]any{"owner": "data"}, SchemaRef: "schema/job.definition"},
			{Op: knowledge.OpPut, Address: address, Value: nil, SchemaRef: "schema/job.runtime", ValueSource: source},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return setup.Repo, commit, address
}

func TestStateRefreshFindsDynamicValueWithoutChangingSnapshot(t *testing.T) {
	repo, commit, address := stateProjectionFixture(t)
	engine := &stateTestEngine{docs: map[knowledge.ObjectID]CompiledDoc{}}
	idx := NewIndexEngine("", func(string, kernel.RepositoryID) (Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	lookup := &stateTestLookup{
		value: map[string]any{"status": "running"},
		basis: knowledge.ObservationBasis{BindingGeneration: "g1", Consistency: knowledge.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-08-27T00:00:00Z"},
	}
	request := retrieval.SearchOf(retrieval.SearchMATCH("running"))
	required, err := idx.RequiresState(repo, commit, request)
	if err != nil || !required {
		t.Fatalf("dynamic text query must select State projection: required=%v err=%v", required, err)
	}
	staticOnly := retrieval.SearchOf(retrieval.SearchClause{
		Op:    retrieval.OpEQ,
		Field: &retrieval.FieldRef{Schema: "schema/job.definition", Aspect: "definition", Path: "owner"},
		Value: "data",
	})
	if required, err := idx.RequiresState(repo, commit, staticOnly); err != nil || required {
		t.Fatalf("Snapshot-only query must not depend on State runtime: required=%v err=%v", required, err)
	}
	first, err := idx.RefreshState(context.Background(), repo, commit, lookup, knowledgeserving.RequestContext{})
	if err != nil || first.Mode != IndexModeRebuild || len(first.Observations) != 1 {
		t.Fatalf("first refresh: %#v %v", first, err)
	}
	result, err := idx.SearchStateAt(repo, commit, request)
	if err != nil || len(result.Hits) != 1 || len(result.Hits[0].Version.Observations) != 1 {
		t.Fatalf("dynamic search: %#v %v", result, err)
	}
	value := result.Hits[0].Knowledge.Value.(map[string]any)
	if value["runtime"].(map[string]any)["status"] != "running" {
		t.Fatalf("search hit did not hydrate from published Serving State: %#v", value)
	}
	raw, err := repo.ReadAddress(address, commit)
	if err != nil || raw.Value != nil {
		t.Fatalf("State refresh must not write observation to Snapshot: %#v %v", raw, err)
	}
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil || head != commit {
		t.Fatalf("State refresh changed Repository HEAD: %s %v", head, err)
	}

	lookup.value = map[string]any{"status": "stopped"}
	lookup.basis.SourceRevision = "r2"
	lookup.basis.ObservedAt = "2026-08-27T00:01:00Z"
	second, err := idx.RefreshState(context.Background(), repo, commit, lookup, knowledgeserving.RequestContext{})
	if err != nil || second.Mode != IndexModeIncremental || second.Revision == first.Revision {
		t.Fatalf("incremental observation refresh: %#v %v", second, err)
	}
	if _, err := idx.SearchStateAtRevision(repo, commit, first.Revision, request); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("stale outer SearchView must not read the new revision: %v", err)
	}
	oldResult, err := idx.SearchStateAt(repo, commit, request)
	if err != nil || len(oldResult.Hits) != 0 {
		t.Fatalf("old dynamic value remained searchable: %#v %v", oldResult, err)
	}
	newResult, err := idx.SearchStateAt(repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("stopped")))
	if err != nil || len(newResult.Hits) != 1 {
		t.Fatalf("new dynamic value not searchable: %#v %v", newResult, err)
	}
}

func TestObservedNullProvesMissingAndFailedRefreshKeepsPublishedRevision(t *testing.T) {
	repo, commit, _ := stateProjectionFixture(t)
	engine := &stateTestEngine{docs: map[knowledge.ObjectID]CompiledDoc{}}
	idx := NewIndexEngine("", func(string, kernel.RepositoryID) (Engine, error) { return engine, nil })
	lookup := &stateTestLookup{
		value: nil,
		basis: knowledge.ObservationBasis{BindingGeneration: "g1", Consistency: knowledge.ObservationLatestOnly, SourceRevision: "r-null", ObservedAt: "2026-08-27T00:00:00Z"},
	}
	first, err := idx.RefreshState(context.Background(), repo, commit, lookup, knowledgeserving.RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	missing := retrieval.SearchOf(retrieval.SearchClause{
		Op:    retrieval.OpMissing,
		Field: &retrieval.FieldRef{Schema: "schema/job.runtime", Aspect: "runtime", Path: "owner"},
	})
	result, err := idx.SearchStateAt(repo, commit, missing)
	if err != nil || len(result.Hits) != 1 {
		t.Fatalf("observed null must be eligible for MISSING: %#v %v", result, err)
	}
	lookup.err = errors.New("runtime unavailable")
	if _, err := idx.RefreshState(context.Background(), repo, commit, lookup, knowledgeserving.RequestContext{}); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("failed lookup must fail refresh honestly: %v", err)
	}
	revision, _, ok := idx.StateView(repo.ID(), commit)
	if !ok || revision != first.Revision {
		t.Fatalf("failed refresh replaced published revision: %q want %q", revision, first.Revision)
	}
}
