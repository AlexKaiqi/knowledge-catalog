package writer_test

import (
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

const representationObjectID objectmodel.ObjectID = "representation/probe"

type representationFixture struct {
	store  snapshot.Store
	reader *reader.Reader
	writer *writer.Writer
	ledger snapshot.CommandLedger
}

func newRepresentationFixture(t *testing.T) representationFixture {
	t.Helper()
	store := testkit.MakeTreeStore(t, "kr://regression/representation")
	registry := snapshot.NewRegistry()
	if err := registry.Add(store); err != nil {
		t.Fatal(err)
	}
	ledger, err := snapshot.NewCommandLedger(nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, ledger)
	if err != nil {
		t.Fatal(err)
	}
	return representationFixture{store: store, reader: reader.NewReader(registry), writer: w, ledger: ledger}
}

func representationRecordPut() objectmodel.Operation {
	return objectmodel.Operation{
		Op: objectmodel.OpPut,
		Address: objectmodel.Address{
			Kind: objectmodel.KindAspect, ObjectID: representationObjectID, AspectName: "health",
		},
		Value: map[string]any{"status": "record"}, PathHint: "probe/health.json",
	}
}

func representationMemberPut() objectmodel.Operation {
	return objectmodel.Operation{
		Op: objectmodel.OpPut,
		Address: objectmodel.Address{
			Kind: objectmodel.KindMember, ObjectID: representationObjectID, AspectName: "health", MemberKey: "primary",
		},
		Value: map[string]any{"status": "member"}, PathHint: "probe/health-primary.json",
	}
}

func (f representationFixture) change(base kernel.CommitID, operations []objectmodel.Operation) objectmodel.ChangeSet {
	return objectmodel.ChangeSet{
		TargetRepository: f.store.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base, Operations: operations,
	}
}

func TestWriterRejectsConflictingFinalAspectRepresentation(t *testing.T) {
	record, member := representationRecordPut(), representationMemberPut()
	cases := []struct {
		name       string
		initial    []objectmodel.Operation
		operations []objectmodel.Operation
	}{
		{name: "same_batch_record_then_member", operations: []objectmodel.Operation{record, member}},
		{name: "same_batch_member_then_record", operations: []objectmodel.Operation{member, record}},
		{name: "existing_record_then_member", initial: []objectmodel.Operation{record}, operations: []objectmodel.Operation{member}},
		{name: "existing_member_then_record", initial: []objectmodel.Operation{member}, operations: []objectmodel.Operation{record}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRepresentationFixture(t)
			before := testkit.MustHead(t, f.store, snapshot.DefaultRef)
			var original objectmodel.ObjectValue
			var seedReceipt writer.IdempotencyEntry
			if len(tc.initial) != 0 {
				receipt, err := f.writer.Commit("seed", f.change(before, tc.initial))
				if err != nil {
					t.Fatal(err)
				}
				before = receipt.Result.NewCommit
				original, err = f.reader.Read(objectmodel.ObjectRef{Repository: f.store.ID(), Object: representationObjectID}, before)
				if err != nil {
					t.Fatal(err)
				}
				var ok bool
				seedReceipt, ok = f.writer.Lookup("seed")
				if !ok {
					t.Fatal("valid seed has no accepted receipt")
				}
			}

			const commandID = "reject-conflicting-representation"
			receipt, err := f.writer.Commit(commandID, f.change(before, tc.operations))
			if kernel.CodeOf(err) != kernel.ErrObjectIDConflict {
				t.Errorf("conflicting final Aspect/Member representation must be rejected with OBJECT_ID_CONFLICT, got %v", err)
			}
			after := testkit.MustHead(t, f.store, snapshot.DefaultRef)
			if after != before {
				t.Errorf("rejected representation advanced published Ref: before=%s after=%s", before, after)
			}
			if receipt.Result.CommitID != "" || receipt.Result.NewCommit != "" || receipt.Disposition != "" {
				t.Errorf("rejected representation returned an accepted receipt: disposition=%s commit=%s newCommit=%s", receipt.Disposition, receipt.Result.CommitID, receipt.Result.NewCommit)
			}
			if entry, ok := f.ledger.Lookup(commandID); ok && entry.Status == snapshot.StatusApplied {
				t.Errorf("rejected representation persisted APPLIED command state for %s", entry.CommandID)
			}
			if entry, ok := f.writer.Lookup(commandID); ok {
				t.Errorf("rejected representation is replayable as an accepted Writer receipt: disposition=%s commit=%s", entry.Receipt.Disposition, entry.Receipt.Result.NewCommit)
			}

			actual, readErr := f.reader.Read(objectmodel.ObjectRef{Repository: f.store.ID(), Object: representationObjectID}, after)
			if len(tc.initial) == 0 {
				if kernel.CodeOf(readErr) != kernel.ErrObjectRefUnresolved {
					t.Errorf("rejected creation must leave object absent, got value=%#v error=%v", actual, readErr)
				}
				return
			}
			if readErr != nil {
				t.Errorf("rejected conflict must leave original object readable: %v", readErr)
			} else if !reflect.DeepEqual(actual, original) {
				t.Errorf("rejected conflict changed original object: before=%#v after=%#v", original, actual)
			}
			if saved, ok := f.writer.Lookup("seed"); !ok || !reflect.DeepEqual(saved, seedReceipt) {
				t.Errorf("rejected conflict changed the accepted seed receipt: before=%#v after=%#v found=%t", seedReceipt, saved, ok)
			}
		})
	}
}

func TestWriterAllowsExplicitAspectRepresentationConversion(t *testing.T) {
	record, member := representationRecordPut(), representationMemberPut()
	cases := []struct {
		name    string
		initial objectmodel.Operation
		next    objectmodel.Operation
		want    any
	}{
		{name: "record_to_member", initial: record, next: member, want: map[string]any{"health": map[string]any{"primary": member.Value}}},
		{name: "member_to_record", initial: member, next: record, want: map[string]any{"health": record.Value}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRepresentationFixture(t)
			root := testkit.MustHead(t, f.store, snapshot.DefaultRef)
			seed, err := f.writer.Commit("seed", f.change(root, []objectmodel.Operation{tc.initial}))
			if err != nil {
				t.Fatal(err)
			}
			operations := []objectmodel.Operation{{Op: objectmodel.OpRemove, Address: tc.initial.Address}, tc.next}
			receipt, err := f.writer.Commit("convert", f.change(seed.Result.NewCommit, operations))
			if err != nil {
				t.Fatalf("explicit removal followed by replacement must allow a consistent representation: %v", err)
			}
			head := testkit.MustHead(t, f.store, snapshot.DefaultRef)
			if head != receipt.Result.NewCommit || head == seed.Result.NewCommit {
				t.Fatalf("successful conversion did not publish its receipt: head=%s receipt=%#v", head, receipt)
			}
			actual, err := f.reader.Read(objectmodel.ObjectRef{Repository: f.store.ID(), Object: representationObjectID}, head)
			if err != nil {
				t.Fatalf("converted final representation must be readable: %v", err)
			}
			if !reflect.DeepEqual(actual.Value, tc.want) {
				t.Errorf("conversion returned wrong complete object: got=%#v want=%#v", actual.Value, tc.want)
			}
		})
	}
}
