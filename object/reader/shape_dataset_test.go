package reader_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	objectservice "kc/object/service"
	"kc/snapshot"
)

// U21 with U7/U16: a shape inside a Dataset selects only what the Dataset
// published; an out-of-scope Aspect is located at its path and its body never
// reaches the result.
func TestShapeInsideDatasetLocatesOutOfScopeAspectWithoutItsBody(t *testing.T) {
	s := testkit.NewSetup(t, "")
	commit, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/A", AspectName: "public"}, PathHint: "public/a.yaml", Value: map[string]any{"body": "public", "extra": 1}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/A", AspectName: "private"}, PathHint: "private/a.yaml", Value: map[string]any{"body": "SECRET_OUTSIDE_DATASET"}},
		}})
	if err != nil {
		t.Fatal(err)
	}
	serving := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return s.Repo.Repository, nil }, reader.DatasetPin{
		SetID: "public-only", Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit},
		Items: []reader.DatasetItem{{Repository: s.RepositoryID, Commit: commit, Kind: "prefix", Prefix: "public"}}})
	executor := objectservice.ShapeReadExecutor{
		Read: func(_ context.Context, ref objectmodel.Reference) (objectmodel.ObservedValue, error) {
			values, err := serving.ReadReference(ref)
			if err != nil {
				return objectmodel.ObservedValue{}, err
			}
			if len(values) != 1 {
				return objectmodel.ObservedValue{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "not published in this Dataset")
			}
			return objectmodel.ObservedValue{FederatedValue: values[0]}, nil
		},
		Schema: func(context.Context, kernel.RepositoryID, kernel.CommitID, string) (*objectmodel.SchemaDefinition, error) {
			return nil, nil
		},
	}
	got, err := executor.Execute(context.Background(), objectservice.ShapeReadRequest{
		Reference: objectmodel.Reference{Repository: s.RepositoryID, Object: "service/A"}, Shape: `{ public { body } private { body } }`,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"public": map[string]any{"body": "public"}, "private": nil}
	if !reflect.DeepEqual(got.Data, want) {
		t.Fatalf("Dataset shape data = %#v", got.Data)
	}
	if len(got.Errors) != 1 || !reflect.DeepEqual(got.Errors[0].Path, []string{"private"}) || got.Errors[0].Code != kernel.ErrObjectRefUnresolved {
		t.Fatalf("out-of-scope Aspect must be located at its path: %#v", got.Errors)
	}
	if len(got.Sources) != 1 || !reflect.DeepEqual(got.Sources[0].Aspects, []string{"public"}) {
		t.Fatalf("sources must cover only the published selection: %#v", got.Sources)
	}
	if raw, _ := json.Marshal(got); strings.Contains(string(raw), "SECRET_OUTSIDE_DATASET") {
		t.Fatalf("out-of-scope body leaked into the shaped result: %s", raw)
	}
}
