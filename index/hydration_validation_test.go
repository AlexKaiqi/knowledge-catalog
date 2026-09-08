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

type malformedSearchHydrator struct {
	mutate func(*knowledge.KnowledgeValue)
}

func (h malformedSearchHydrator) ReadMany(repo knowledge.Repository, commit kernel.CommitID, ids []knowledge.ObjectID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	values := make(map[knowledge.ObjectID]knowledge.KnowledgeValue, len(ids))
	for _, id := range ids {
		value, err := repo.Read(id, commit)
		if err != nil {
			return nil, err
		}
		h.mutate(&value)
		values[id] = value
	}
	return values, nil
}

func (h malformedSearchHydrator) ReadAddress(repo knowledge.Repository, commit kernel.CommitID, address knowledge.Address) (knowledge.KnowledgeValue, error) {
	return repo.ReadAddress(address, commit)
}

func TestSearchRejectsMalformedInjectedHydration(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/hydration-validation")
	commit := putAt(t, repo, testkit.MustHead(t, repo, snapshot.DefaultRef), []knowledge.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "runbook alpha"}, "")[0],
	})
	engine := &staleCandidateEngine{candidates: []index.CandidateRef{{Repository: repo.ID(), ObjectID: "policy/P-1", Basis: commit}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, commit); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*knowledge.KnowledgeValue){
		"repository":     func(v *knowledge.KnowledgeValue) { v.Repository = "kr://other" },
		"ref-repository": func(v *knowledge.KnowledgeValue) { v.KnowledgeRef.Repository = "kr://other" },
		"ref-object":     func(v *knowledge.KnowledgeValue) { v.KnowledgeRef.Object = "policy/other" },
		"commit":         func(v *knowledge.KnowledgeValue) { v.Commit = "other-commit" },
		"object":         func(v *knowledge.KnowledgeValue) { v.Address.ObjectID = "policy/other" },
		"aspect-as-object": func(v *knowledge.KnowledgeValue) {
			v.Address.Kind, v.Address.AspectName = knowledge.KindAspect, "body"
		},
		"foreign-unit": func(v *knowledge.KnowledgeValue) {
			v.Units = []knowledge.Address{{Kind: knowledge.KindAspect, ObjectID: "policy/other", AspectName: "body"}}
		},
		"foreign-declaration": func(v *knowledge.KnowledgeValue) {
			v.Declarations = []knowledge.UnitDeclaration{{Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "policy/other", AspectName: "body"}}}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			idx.SetHydrator(malformedSearchHydrator{mutate: mutate})
			if result, err := idx.SearchAt(repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("runbook"))); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("malformed hydration became a search result: %+v %v", result, err)
			}
		})
	}
}
