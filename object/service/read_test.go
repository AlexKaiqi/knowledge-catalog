package service

import (
	"context"
	"reflect"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

type readRecorder struct {
	ref    objectmodel.Reference
	commit kernel.CommitID
	calls  int
}

func TestApplicationRequestsUseOwnedIdentifierTypes(t *testing.T) {
	typ := reflect.TypeOf(ReadRequest{})
	want := map[string]reflect.Type{
		"Reference": reflect.TypeOf(objectmodel.Reference{}),
		"Commit":    reflect.TypeOf(kernel.CommitID("")),
	}
	for field, expected := range want {
		actual, ok := typ.FieldByName(field)
		if !ok || actual.Type != expected {
			t.Fatalf("ReadRequest.%s type = %v, want %v", field, actual.Type, expected)
		}
	}
}

func (r *readRecorder) ReadReference(ref objectmodel.Reference, commit kernel.CommitID) (objectmodel.KnowledgeValue, error) {
	r.ref, r.commit = ref, commit
	r.calls++
	return objectmodel.KnowledgeValue{KnowledgeRef: objectmodel.KnowledgeRef{Repository: ref.Repository, Object: ref.Object}, Commit: commit, Locator: ref.Locator}, nil
}

func TestReadExecutorUsesTypedRepositoryObjectAndBasis(t *testing.T) {
	recorder := &readRecorder{}
	request := ReadRequest{
		Reference: objectmodel.Reference{Repository: "kr://app/read", Object: "policy/A", Locator: "/owner/team"},
		Commit:    kernel.CommitID("fixed"),
	}
	result, err := (ReadExecutor{Reader: recorder}).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if recorder.ref != request.Reference || recorder.calls != 1 || recorder.commit != request.Commit || result.Commit != request.Commit || result.Locator != request.Reference.Locator {
		t.Fatalf("typed read coordinates were not preserved: %#v %#v", recorder, result)
	}
}

func TestReadExecutorRejectsIncompleteReferenceBeforeReading(t *testing.T) {
	for _, request := range []ReadRequest{
		{Commit: "fixed"},
		{Reference: objectmodel.Reference{Repository: "kr://app/read"}, Commit: "fixed"},
		{Reference: objectmodel.Reference{Object: "policy/A"}, Commit: "fixed"},
		{Reference: objectmodel.Reference{Repository: "kr://app/read", Object: "policy/A", Locator: "/~3bad"}, Commit: "fixed"},
		{Reference: objectmodel.Reference{Repository: "kr://app/read", Object: "policy/A"}},
	} {
		recorder := &readRecorder{}
		_, err := (ReadExecutor{Reader: recorder}).Execute(context.Background(), request)
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid || recorder.calls != 0 {
			t.Fatalf("incomplete reference reached the reader: request=%#v error=%v calls=%d", request, err, recorder.calls)
		}
	}
}

func TestReadExecutorRequiresTheReferenceReadPort(t *testing.T) {
	_, err := (ReadExecutor{}).Execute(context.Background(), ReadRequest{
		Reference: objectmodel.Reference{Repository: "kr://app/read", Object: "policy/A"}, Commit: "fixed",
	})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing reference read service: %v", err)
	}
}
