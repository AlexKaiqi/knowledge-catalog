package projection

import (
	"strings"
	"testing"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type adoptionMatchVerifier struct{}

func (adoptionMatchVerifier) VerifyResidualMatch(text, query string, _ retrieval.MatchMode) (bool, error) {
	return strings.Contains(text, query), nil
}

func TestDiscoveryUsesEachUnitsActuallyAdoptedSchemaAccess(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://test/schema-adoption")
	commit := func(operations ...objectmodel.Operation) kernel.CommitID {
		t.Helper()
		base := testkit.MustHead(t, repo, snapshot.DefaultRef)
		at, err := repo.ApplyKnowledgeCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
			BaseCommit: base, ExpectedTargetCommit: base, Operations: operations})
		if err != nil {
			t.Fatal(err)
		}
		return at
	}
	schema := func(current bool) objectmodel.Operation {
		title, tag := []string{"text"}, []string{}
		if current {
			title, tag = []string{"filter"}, []string{"filter"}
		}
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/asset"},
			Value: map[string]any{"entity": "Asset", "fields": map[string]any{
				"title": map[string]any{"type": "string", "access": title}, "tag": map[string]any{"type": "string", "access": tag},
			}}}
	}
	old := commit(schema(false))
	head := commit(schema(true),
		objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "asset/old"},
			SchemaRef: "schema/asset@" + string(old), Value: map[string]any{"title": "oldtoken", "tag": "hidden"}},
		objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "asset/old-missing"},
			SchemaRef: "schema/asset@" + string(old), Value: map[string]any{}},
		objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "asset/current"},
			SchemaRef: "schema/asset", Value: map[string]any{"title": "current", "tag": "visible"}})
	spec, err := specAtCommit(repo, head)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.HasHint(reader.HintText) || !spec.HasHint(reader.HintFilter) {
		t.Errorf("discovery omitted a currently adopted access contract: %#v", spec)
	}
	retained := spec.AdoptionWitnesses()
	verified, err := retrieval.AccessSpecFromAdoptions(repo, head, retained)
	if err != nil || verified.AccessDigest != spec.AccessDigest {
		t.Fatalf("retained adoption did not reproduce the authoritative access contract: %#v %v", verified, err)
	}
	if len(retained) != 1 {
		t.Fatalf("expected one distinct adopted release: %#v", retained)
	}
	for _, invalid := range []retrieval.SchemaAdoption{
		{SchemaRef: "schema/asset@forged-version", Witnesses: retained[0].Witnesses},
		{SchemaRef: retained[0].SchemaRef, Witnesses: []objectmodel.Address{{Kind: objectmodel.KindEntity, ObjectID: "asset/current"}}},
		{SchemaRef: retained[0].SchemaRef},
	} {
		_, err := retrieval.AccessSpecFromAdoptions(repo, head, []retrieval.SchemaAdoption{invalid})
		if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Errorf("forged Schema adoption was accepted: %#v %v", invalid, err)
		}
	}
	for _, id := range []objectmodel.ObjectID{"asset/old", "asset/old-missing", "asset/current"} {
		value, err := repo.Read(id, head)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := compileProjectionDocument(repo, value, spec)
		if err != nil {
			t.Fatal(err)
		}
		if id == "asset/current" {
			if doc.Text != "" || len(doc.EligibleFields) != 2 {
				t.Fatalf("current adoption did not supply its filter-only contract: %#v", doc)
			}
			continue
		}
		if len(doc.EligibleFields) != 0 || (id == "asset/old" && doc.Text != "oldtoken") {
			t.Errorf("new Schema access leaked into old adoption: %#v", doc)
		}
		field := &retrieval.FieldRef{Schema: "schema/asset", Path: "title"}
		for _, clause := range []retrieval.SearchClause{{Op: retrieval.OpMissing, Field: field}, {Op: retrieval.OpExists, Field: field},
			{Op: retrieval.OpEQ, Field: field, Value: "oldtoken"}} {
			matched, err := retrieval.MatchesResidual(repo, value, nil, retrieval.SearchOf(clause), spec)
			if err != nil || matched {
				t.Errorf("undeclared filter capability matched %s on %s: %v %v", clause.Op, id, matched, err)
			}
		}
		if id == "asset/old" {
			matched, err := retrieval.MatchesResidual(repo, value, nil, retrieval.SearchOf(retrieval.SearchClause{
				Op: retrieval.OpMatch, Field: field, Value: "oldtoken"}), spec, adoptionMatchVerifier{})
			if err != nil || !matched {
				t.Errorf("old text access did not survive current Schema evolution: %v %v", matched, err)
			}
		}
	}
}
