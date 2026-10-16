package liveaccess_test

import (
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// Finding one healthy descriptor holder is insufficient when another possible
// holder has no readable HEAD. Unknown is not proof that the reference is absent.
func TestDescriptorDependencyFreezePreservesPossibleHolderFailure(t *testing.T) {
	for _, name := range []string{"Head error", "empty Head"} {
		t.Run(name, func(t *testing.T) {
			f := newDescriptorDependencyFixture(t)
			// This ID sorts after the healthy holder, so the ambiguity check
			// has already found a valid descriptor when the failure occurs.
			unknown := testkit.MakeTreeStore(t, "kr://dependency/zz-unavailable")
			if err := f.registry.Add(unknown); err != nil {
				t.Fatal(err)
			}
			repo, err := f.lookup(unknown.ID())
			if err != nil {
				t.Fatal(err)
			}
			fault := &descriptorDependencyHeadFault{Repository: repo}
			wantCode := kernel.ErrPreconditionFailed
			if name == "Head error" {
				fault.failure = kernel.Fail(kernel.ErrTemporaryUnavailable, "possible descriptor holder HEAD is unavailable")
				wantCode = kernel.ErrTemporaryUnavailable
			}
			lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
				if id == unknown.ID() {
					return fault, nil
				}
				return f.lookup(id)
			}
			dependencies, err := reader.Open(lookup, f.pin(nil)).FreezeDependencies(f.registry.IDs)
			if kernel.CodeOf(err) != wantCode || dependencies != nil {
				t.Fatalf("unknown holder was treated as absent: dependencies=%v error=%v want=%s", dependencies, err, wantCode)
			}
			if fault.failure != nil && err != fault.failure {
				t.Fatalf("HEAD failure was replaced: got=%v want=%v", err, fault.failure)
			}
			if fault.failure == nil && (!strings.Contains(err.Error(), string(unknown.ID())) || !strings.Contains(err.Error(), "ops/warehouse-stats")) {
				t.Fatalf("empty HEAD failure lost the repository or descriptor: %v", err)
			}
			if fault.headCalls != 1 || fault.bodyCalls != 0 {
				t.Fatalf("failed HEAD caused body inspection: head=%d body=%d", fault.headCalls, fault.bodyCalls)
			}
		})
	}
}

type descriptorDependencyHeadFault struct {
	objectmodel.Repository
	failure   error
	headCalls int
	bodyCalls int
}

func (r *descriptorDependencyHeadFault) Head(string) (kernel.CommitID, error) {
	r.headCalls++
	return "", r.failure
}

func (r *descriptorDependencyHeadFault) Read(objectmodel.ObjectID, kernel.CommitID) (objectmodel.ObjectValue, error) {
	r.bodyCalls++
	return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrPreconditionFailed, "body must not be read without a fixed source HEAD")
}
