package writer_test

import (
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

var maintenanceDescription = objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "description"}
var maintenanceID = objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "id"}
var maintenanceStatus = objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "status"}

func maintenanceObjectValue() map[string]any {
	return map[string]any{
		"description": map[string]any{"text": "orders"},
		"columns": map[string]any{
			"id":     map[string]any{"type": "integer", "nullable": false, "description": "old field"},
			"status": map[string]any{"type": "string"},
		},
	}
}

func newMaintenanceLifecycleSetup(t *testing.T) (testkit.Setup, kernel.CommitID) {
	t.Helper()
	s := testkit.NewSetup(t, "kr://writer/maintenance-lifecycle")
	value := maintenanceObjectValue()
	columns := value["columns"].(map[string]any)
	receipt, err := s.Writer.Commit("seed", objectmodel.ChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: maintenanceDescription, Value: value["description"]},
			{Op: objectmodel.OpPut, Address: maintenanceID, Value: columns["id"]},
			{Op: objectmodel.OpPut, Address: maintenanceStatus, Value: columns["status"]},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/kept"}, Value: map[string]any{"text": "kept"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, receipt.Result.NewCommit
}

func assertMaintenanceObject(t *testing.T, s testkit.Setup, at kernel.CommitID, objectID objectmodel.ObjectID, want any) {
	t.Helper()
	value, err := s.Reader.Read(objectmodel.KnowledgeRef{Repository: s.RepositoryID, Object: objectID}, at, nil)
	if err != nil {
		t.Fatal(err)
	}
	if value.Commit != at || value.KnowledgeRef.Object != objectID || !reflect.DeepEqual(value.Value, want) {
		t.Fatalf("fixed object read: got %#v, want object=%s commit=%s value=%#v", value, objectID, at, want)
	}
}

func TestMemberReplacementUsesReadDigestAndPreservesSiblingUnits(t *testing.T) {
	s, before := newMaintenanceLifecycleSetup(t)
	read, err := s.Reader.ReadAddress(s.RepositoryID, maintenanceID, before)
	if err != nil {
		t.Fatal(err)
	}
	if read.Commit != before || len(read.Declarations) != 1 || read.Declarations[0].Address != maintenanceID || read.Declarations[0].Digest == "" {
		t.Fatalf("Member READ must carry its fixed content precondition: %#v", read)
	}
	digest := read.Declarations[0].Digest
	replacement := map[string]any{"type": "string"}
	receipt, err := s.Writer.Commit("replace-member", objectmodel.ChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: before, ExpectedTargetCommit: before,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut, Address: maintenanceID, Value: replacement,
			Precondition: &objectmodel.Precondition{Type: objectmodel.IfDigestEquals, Digest: digest},
		}},
	})
	if err != nil {
		t.Fatalf("READ content digest must be usable directly for Member PUT: %v", err)
	}
	want := maintenanceObjectValue()
	want["columns"].(map[string]any)["id"] = map[string]any{"type": "string"}
	assertMaintenanceObject(t, s, receipt.Result.NewCommit, maintenanceID.ObjectID, want)

	// The basis is current, so only the now-stale Member content digest can
	// reject this update. Merely accepting the first PUT would not prove CAS.
	_, err = s.Writer.Commit("stale-member", objectmodel.ChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: receipt.Result.NewCommit, ExpectedTargetCommit: receipt.Result.NewCommit,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut, Address: maintenanceID, Value: map[string]any{"type": "integer"},
			Precondition: &objectmodel.Precondition{Type: objectmodel.IfDigestEquals, Digest: digest},
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
	if head := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); head != receipt.Result.NewCommit {
		t.Fatalf("stale Member precondition advanced Ref: got %s, want %s", head, receipt.Result.NewCommit)
	}
	assertMaintenanceObject(t, s, receipt.Result.NewCommit, maintenanceID.ObjectID, want)
}

func TestMaintenanceUnitRemovalPreservesSiblingUnits(t *testing.T) {
	for _, tc := range []struct {
		name    string
		address objectmodel.Address
	}{
		{name: "member", address: maintenanceID},
		{name: "complete_aspect", address: maintenanceDescription},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, before := newMaintenanceLifecycleSetup(t)
			receipt, err := s.Writer.Commit("remove-unit", objectmodel.ChangeSet{
				TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
				BaseCommit: before, ExpectedTargetCommit: before,
				Operations: []objectmodel.Operation{{Op: objectmodel.OpRemove, Address: tc.address}},
			})
			if err != nil {
				t.Fatal(err)
			}
			want := maintenanceObjectValue()
			if tc.address.MemberKey == "" {
				delete(want, tc.address.AspectName)
			} else {
				delete(want[tc.address.AspectName].(map[string]any), tc.address.MemberKey)
			}
			assertMaintenanceObject(t, s, receipt.Result.NewCommit, tc.address.ObjectID, want)
			if _, err := s.Reader.ReadAddress(s.RepositoryID, tc.address, receipt.Result.NewCommit); kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
				t.Fatalf("removed unit is still readable: %v", err)
			}
			assertMaintenanceObject(t, s, receipt.Result.NewCommit, "note/kept", map[string]any{"text": "kept"})
		})
	}
}

func TestObjectRemovalDeletesAllUnitsAndPreservesFixedHistory(t *testing.T) {
	s, before := newMaintenanceLifecycleSetup(t)
	wantBefore := maintenanceObjectValue()
	receipt, err := s.Writer.Commit("remove-object", objectmodel.ChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: before, ExpectedTargetCommit: before,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: maintenanceID.ObjectID},
			Precondition: &objectmodel.Precondition{Type: objectmodel.IfDigestEquals, Digest: kernel.CanonicalDigest(wantBefore)},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if head := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); head != receipt.Result.NewCommit || head == before {
		t.Fatalf("accepted removal did not publish its new version: head=%s receipt=%#v", head, receipt)
	}
	if value, err := s.Reader.Read(objectmodel.KnowledgeRef{Repository: s.RepositoryID, Object: maintenanceID.ObjectID}, receipt.Result.NewCommit, nil); kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		t.Errorf("removed object must fail READ rather than return content: value=%#v error=%v", value, err)
	}
	for _, address := range []objectmodel.Address{maintenanceDescription, maintenanceID, maintenanceStatus} {
		if _, err := s.Reader.ReadAddress(s.RepositoryID, address, receipt.Result.NewCommit); kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
			t.Errorf("object removal retained unit %#v: %v", address, err)
		}
	}
	assertMaintenanceObject(t, s, before, maintenanceID.ObjectID, wantBefore)
	assertMaintenanceObject(t, s, receipt.Result.NewCommit, "note/kept", map[string]any{"text": "kept"})
}
