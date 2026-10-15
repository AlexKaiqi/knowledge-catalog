// Package testfixture shares concrete-plugin test material through generic ports.
// It never constructs an Index or Controller.
package testfixture

import (
	"context"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectserving "kc/liveaccess"
	objectmodel "kc/object"
	"kc/snapshot"
	"sort"
	"strings"
	"testing"
	"unicode"
)

type StateEngine struct {
	Meta ports.Meta
	Docs map[objectmodel.ObjectID]ports.CompiledDoc
}

func (e *StateEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) ports.Capability {
	return ports.Capability{Guarantee: ports.GuaranteeSuperset, Coverage: 1}
}

func (e *StateEngine) Retrieve(ports.RetrieveRequest) (ports.CandidatePage, error) {
	ids := make([]objectmodel.ObjectID, 0, len(e.Docs))
	for id := range e.Docs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	page := ports.CandidatePage{Exhausted: true}
	for _, id := range ids {
		page.Candidates = append(page.Candidates, ports.CandidateRef{ObjectID: id, Basis: e.Meta.Basis})
	}
	return page, nil
}

func (e *StateEngine) LoadMeta() (ports.Meta, error) { return e.Meta, nil }

func (e *StateEngine) Rebuild(docs []ports.CompiledDoc, meta ports.Meta) error {
	e.Docs = map[objectmodel.ObjectID]ports.CompiledDoc{}
	for _, doc := range docs {
		e.Docs[doc.ObjectID] = doc
	}
	e.Meta = meta
	return nil
}

func (e *StateEngine) Apply(upserts []ports.CompiledDoc, deletes []objectmodel.ObjectID, meta ports.Meta) error {
	for _, id := range deletes {
		delete(e.Docs, id)
	}
	for _, doc := range upserts {
		e.Docs[doc.ObjectID] = doc
	}
	e.Meta = meta
	return nil
}

func (e *StateEngine) Count() (int, error) { return len(e.Docs), nil }

func (e *StateEngine) Close() error { return nil }

func (e *StateEngine) BeginRebuild(meta ports.Meta) (ports.RebuildSession, error) {
	return &stateRebuildSession{engine: e, meta: meta}, nil
}

type stateRebuildSession struct {
	engine *StateEngine
	meta   ports.Meta
	docs   []ports.CompiledDoc
}

func (s *stateRebuildSession) Append(docs []ports.CompiledDoc) error {
	s.docs = append(s.docs, docs...)
	return nil
}

func (s *stateRebuildSession) Commit() error { return s.engine.Rebuild(s.docs, s.meta) }

func (s *stateRebuildSession) Abort(err error) error {
	return err
}

type StateLookup struct {
	Value any
	Basis objectmodel.ObservationBasis
	Err   error
	Calls int
}

func (s *StateLookup) LookupState(context.Context, objectserving.StateLookupRequest) (objectserving.StateObservation, error) {
	s.Calls++
	if s.Err != nil {
		return objectserving.StateObservation{}, s.Err
	}
	return objectserving.StateObservation{Value: s.Value, Basis: s.Basis}, nil
}

func StateProjection(t *testing.T) (objectmodel.Repository, kernel.CommitID, objectmodel.Address) {
	t.Helper()
	setup := testkit.NewSetup(t, "")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/orders", AspectName: "runtime"}
	commit, err := setup.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: setup.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: setup.RootCommitID, ExpectedTargetCommit: setup.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.definition"}, Value: map[string]any{
				"entity": "Job", "aspect": "definition", "fields": map[string]any{"owner": map[string]any{"type": "string", "access": []any{"filter"}}},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, Value: map[string]any{
				"entity": "Job", "aspect": "runtime", "origin": "https://scheduler.example", "fields": map[string]any{
					"status": map[string]any{"type": "string", "access": []any{"text", "filter"}},
					"owner":  map[string]any{"type": "string", "access": []any{"filter"}},
				},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: address.ObjectID, AspectName: "definition"}, Value: map[string]any{"owner": "data"}, SchemaRef: "schema/job.definition"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return setup.Repo, commit, address
}

func (*StateEngine) VerifyResidualMatch(text, query string, mode retrieval.MatchMode) (bool, error) {
	tokenize := func(raw string) []string {
		return strings.FieldsFunc(strings.ToLower(raw), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	}
	terms, wanted := tokenize(text), tokenize(query)
	if len(wanted) == 0 {
		return false, nil
	}
	if mode == retrieval.MatchPhrase {
		for i := 0; i+len(wanted) <= len(terms); i++ {
			if strings.Join(terms[i:i+len(wanted)], "\x00") == strings.Join(wanted, "\x00") {
				return true, nil
			}
		}
		return false, nil
	}
	for _, want := range wanted {
		found := false
		for _, term := range terms {
			if term == want {
				found = true
				break
			}
		}
		if mode == retrieval.MatchAnyTerms && found {
			return true, nil
		}
		if mode != retrieval.MatchAnyTerms && !found {
			return false, nil
		}
	}
	return mode != retrieval.MatchAnyTerms, nil
}
