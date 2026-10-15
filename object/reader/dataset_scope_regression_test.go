package reader_test

import (
	"fmt"
	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
	"testing"
)

type privateBindingGuard struct {
	objectmodel.Repository
	objectmodel.UnitLocator
	objectmodel.SchemaStore
}

func (privateBindingGuard) BindingSchemaObjectIDs(kernel.CommitID) ([]objectmodel.ObjectID, error) {
	return nil, fmt.Errorf("unscoped binding locator would read private schema bodies")
}

func TestDatasetRegressionDatasetMustNotReadOutsideAspect(t *testing.T) {
	s := testkit.NewSetup(t, "")
	commit, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/A", AspectName: "public"}, PathHint: "public/a.yaml", Value: map[string]any{"body": "public"}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/A", AspectName: "private"}, PathHint: "private/a.yaml", Value: map[string]any{"body": "SECRET_OUTSIDE_DATASET"}},
		}})
	if err != nil {
		t.Fatal(err)
	}
	serving := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return s.Repo.Repository, nil }, reader.DatasetPin{
		SetID: "public-only", Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit},
		Items: []reader.DatasetItem{{Repository: s.RepositoryID, Commit: commit, Kind: "prefix", Prefix: "public"}}})
	rows, err := serving.ReadAddress(objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/A", AspectName: "private"})
	if err == nil && len(rows) > 0 {
		t.Fatalf("outside private Aspect returned: %#v", rows[0].Value)
	}
	publicAddress := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/A", AspectName: "public"}
	if rows, err := serving.ReadAddress(publicAddress); err != nil || len(rows) != 1 {
		t.Fatalf("published exact Address must remain readable: %#v %v", rows, err)
	}
	if rows, err := serving.ResolveAddress(publicAddress); err != nil || len(rows) != 1 {
		t.Fatalf("published exact Address must remain resolvable: %#v %v", rows, err)
	}
	if rows, err := serving.Read("service/A", nil); err != nil || len(rows) != 0 {
		t.Fatalf("partial object passed as complete Canonical: %#v %v", rows, err)
	}
	if contains, err := serving.Contains(s.RepositoryID, "service/A"); err != nil || contains {
		t.Fatalf("partial object eligible for search: %v %v", contains, err)
	}
	full := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return s.Repo, nil }, reader.DatasetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit}, Items: reader.WholeRepositoryItems(map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit})})
	if rows, err := full.Read("service/A", nil); err != nil || len(rows) != 1 {
		t.Fatalf("complete publication cannot read object: %#v %v", rows, err)
	}
}

func TestDatasetMemberCannotReadAuxiliaryObjectOutsideScope(t *testing.T) {
	s := testkit.NewSetup(t, "")
	commit, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "fixture/public"}, PathHint: "public/a.yaml", Value: map[string]any{"body": "visible"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/private"}, PathHint: "private/schema.yaml", Value: map[string]any{"entity": "Private", "fields": map[string]any{"secret": map[string]any{"type": "string", "access": []any{"filter"}}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	pin := reader.DatasetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit}, Items: []reader.DatasetItem{{Repository: s.RepositoryID, Commit: commit, Kind: "prefix", Prefix: "public"}}}
	guard := privateBindingGuard{Repository: s.Repo, UnitLocator: s.Repo.Repository.(objectmodel.UnitLocator), SchemaStore: s.Repo.Repository.(objectmodel.SchemaStore)}
	scope := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return guard, nil }, pin)
	pin.Items[0].Prefix = "" // caller mutation must not broaden the accepted pin
	copy := scope.Pin()
	copy.Items[0].Prefix = ""
	member, err := scope.Member(s.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := member.Read("fixture/public", commit); err != nil {
		t.Fatal(err)
	}
	if _, err := member.Read("schema/private", commit); err == nil {
		t.Fatal("auxiliary read escaped scope")
	}
	if _, err := member.Read("fixture/public", s.RootCommitID); err == nil {
		t.Fatal("auxiliary read escaped version")
	}
	ids, err := member.(objectmodel.SchemaStore).SchemaObjectIDs(commit)
	if err != nil || len(ids) != 0 {
		t.Fatalf("schema namespace escaped scope: %v %v", ids, err)
	}
	bound, err := member.(objectmodel.BindingLocator).BindingSchemaObjectIDs(commit)
	if err != nil || len(bound) != 0 {
		t.Fatalf("binding discovery escaped scope: %v %v", bound, err)
	}
	plan, err := retrieval.PlanAccess(func(kernel.RepositoryID) (objectmodel.Repository, error) { return s.Repo, nil }, scope.Pin())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Specs) != 1 || len(plan.Specs[0].Schemas) != 0 {
		t.Fatalf("access plan exposed unpublished schemas: %#v", plan)
	}
}
