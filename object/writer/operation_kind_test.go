package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestUnsupportedOperationCannotBecomeUnitReplacementOrRemoval(t *testing.T) {
	for _, kind := range []objectmodel.OpKind{"", "PATCH", "APPEND"} {
		t.Run(string(kind), func(t *testing.T) {
			s, before := newMaintenanceLifecycleSetup(t)
			_, err := s.Writer.Commit("invalid-operation", objectmodel.ChangeSet{
				TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
				BaseCommit: before, ExpectedTargetCommit: before,
				Operations: []objectmodel.Operation{{Op: kind, Address: maintenanceID, Value: map[string]any{"type": "changed"}}},
			})
			if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
				t.Fatalf("unsupported operation was treated as PUT or REMOVE: %v", err)
			}
			if head := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); head != before {
				t.Fatalf("unsupported operation advanced Ref: %s -> %s", before, head)
			}
			assertMaintenanceObject(t, s, before, maintenanceID.ObjectID, maintenanceObjectValue())
		})
	}
}
