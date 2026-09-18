package knowledgeapp

import (
	"context"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

type readRecorder struct {
	ref     knowledge.KnowledgeRef
	address *knowledge.Address
	commit  kernel.CommitID
}

func TestApplicationRequestsUseOwnedIdentifierTypes(t *testing.T) {
	typ := reflect.TypeOf(ReadRequest{})
	want := map[string]reflect.Type{
		"Repository": reflect.TypeOf(kernel.RepositoryID("")),
		"Commit":     reflect.TypeOf(kernel.CommitID("")),
		"Object":     reflect.TypeOf(knowledge.ObjectID("")),
	}
	for field, expected := range want {
		actual, ok := typ.FieldByName(field)
		if !ok || actual.Type != expected {
			t.Fatalf("ReadRequest.%s type = %v, want %v", field, actual.Type, expected)
		}
	}
}

func (r *readRecorder) Read(ref knowledge.KnowledgeRef, commit kernel.CommitID, _ *knowledge.AspectSelector) (knowledge.KnowledgeValue, error) {
	r.ref, r.commit = ref, commit
	return knowledge.KnowledgeValue{KnowledgeRef: ref, Commit: commit}, nil
}

func (r *readRecorder) ReadAddress(repository kernel.RepositoryID, address knowledge.Address, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	r.address, r.commit = &address, commit
	return knowledge.KnowledgeValue{Repository: repository, Address: address, Commit: commit}, nil
}

func TestReadExecutorUsesTypedRepositoryObjectAndBasis(t *testing.T) {
	recorder := &readRecorder{}
	request := ReadRequest{
		Repository: kernel.RepositoryID("kr://app/read"),
		Commit:     kernel.CommitID("fixed"),
		Object:     knowledge.ObjectID("policy/A"),
	}
	result, err := (ReadExecutor{Reader: recorder}).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if recorder.ref.Repository != request.Repository || recorder.ref.Object != request.Object ||
		recorder.commit != request.Commit || result.Commit != request.Commit {
		t.Fatalf("typed read coordinates were not preserved: %#v %#v", recorder, result)
	}
}

func TestReadExecutorRejectsMismatchedTypedAddress(t *testing.T) {
	recorder := &readRecorder{}
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "policy/B", AspectName: "owner"}
	_, err := (ReadExecutor{Reader: recorder}).Execute(context.Background(), ReadRequest{
		Repository: "kr://app/read", Commit: "fixed", Object: "policy/A", Address: &address,
	})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("mismatched address = %v", err)
	}
}
