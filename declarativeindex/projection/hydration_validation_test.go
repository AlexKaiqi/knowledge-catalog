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

type malformedSearchHydrator struct {
	mutate func(*objectmodel.ObjectValue)
}

func (h malformedSearchHydrator) ReadMany(repo objectmodel.Repository, commit kernel.CommitID, ids []objectmodel.ObjectID) (map[objectmodel.ObjectID]objectmodel.ObjectValue, error) {
	values := make(map[objectmodel.ObjectID]objectmodel.ObjectValue, len(ids))
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

func (h malformedSearchHydrator) ReadAddress(repo objectmodel.Repository, commit kernel.CommitID, address objectmodel.Address) (objectmodel.ObjectValue, error) {
	return repo.ReadAddress(address, commit)
}

func TestSearchRejectsMalformedInjectedHydration(t *testing.T) {
	repo := testfixture.MakeIndexRepository(t, "kr://acme/public/hydration-validation")
	commit := testfixture.PutAt(t, repo, testkit.MustHead(t, repo, snapshot.DefaultRef), []objectmodel.Operation{
		testfixture.PolicyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "runbook alpha"}, "")[0],
	})
	engine := &staleCandidateEngine{candidates: []ports.CandidateRef{{Repository: repo.ID(), ObjectID: "policy/P-1", Basis: commit}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, commit); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*objectmodel.ObjectValue){
		"repository":     func(v *objectmodel.ObjectValue) { v.Repository = "kr://other" },
		"ref-repository": func(v *objectmodel.ObjectValue) { v.ObjectRef.Repository = "kr://other" },
		"ref-object":     func(v *objectmodel.ObjectValue) { v.ObjectRef.Object = "policy/other" },
		"commit":         func(v *objectmodel.ObjectValue) { v.Commit = "other-commit" },
		"object":         func(v *objectmodel.ObjectValue) { v.Address.ObjectID = "policy/other" },
		"aspect-as-object": func(v *objectmodel.ObjectValue) {
			v.Address.Kind, v.Address.AspectName = objectmodel.KindAspect, "body"
		},
		"foreign-unit": func(v *objectmodel.ObjectValue) {
			v.Units = []objectmodel.Address{{Kind: objectmodel.KindAspect, ObjectID: "policy/other", AspectName: "body"}}
		},
		"foreign-declaration": func(v *objectmodel.ObjectValue) {
			v.Declarations = []objectmodel.UnitDeclaration{{Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/other", AspectName: "body"}}}
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
