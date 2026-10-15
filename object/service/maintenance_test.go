package service_test

import (
	"context"
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/service"
	"kc/object/writer"
	"kc/snapshot"
)

func TestReferenceMaintenanceUsesActualUnitModelAndPreservesSiblings(t *testing.T) {
	s := testkit.NewSetup(t, "kr://object/reference-maintenance")
	member := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "id"}
	seed, err := s.Writer.Commit("seed", objectmodel.ChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: member, Value: map[string]any{"type": "integer", "old": true}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "status"}, Value: "kept"},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "description"}, Value: map[string]any{"nested": "whole aspect"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	e := service.MaintenanceExecutor{Writer: s.Writer, Repository: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return s.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}}
	ref, _ := objectmodel.FormatReference(objectmodel.Reference{Repository: s.RepositoryID, Object: "table/orders", Locator: "/columns/id"})
	read, err := s.Reader.ReadAddress(s.RepositoryID, member, seed.Result.NewCommit)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := e.Execute(context.Background(), objectmodel.OpPut, service.MaintenanceRequest{
		Reference: ref, Repository: s.RepositoryID, CommandID: "replace", Base: seed.Result.NewCommit,
		Value: map[string]any{"type": "string"}, IfDigest: read.Declarations[0].Digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Reader.Read(objectmodel.ObjectRef{Repository: s.RepositoryID, Object: "table/orders"}, updated.Result.NewCommit, nil)
	want := map[string]any{"columns": map[string]any{"id": map[string]any{"type": "string"}, "status": "kept"}, "description": map[string]any{"nested": "whole aspect"}}
	if err != nil || !reflect.DeepEqual(got.Value, want) {
		t.Fatalf("reference PUT changed wrong content: got=%#v error=%v", got.Value, err)
	}
	for _, locator := range []string{"/columns", "/description/nested", "/columns/id/type"} {
		bad, _ := objectmodel.FormatReference(objectmodel.Reference{Repository: s.RepositoryID, Object: "table/orders", Locator: locator})
		_, err := e.Execute(context.Background(), objectmodel.OpRemove, service.MaintenanceRequest{Reference: bad, Repository: s.RepositoryID, CommandID: "reject-" + locator})
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != updated.Result.NewCommit {
			t.Fatalf("non-maintenance locator %s must refuse without publishing: %v", locator, err)
		}
	}
	removed, err := e.Execute(context.Background(), objectmodel.OpRemove, service.MaintenanceRequest{Reference: ref, Repository: s.RepositoryID, CommandID: "remove-member", Base: updated.Result.NewCommit})
	if err != nil {
		t.Fatal(err)
	}
	value, err := s.Reader.Read(objectmodel.ObjectRef{Repository: s.RepositoryID, Object: "table/orders"}, removed.Result.NewCommit, nil)
	if err != nil || !reflect.DeepEqual(value.Value, map[string]any{"columns": map[string]any{"status": "kept"}, "description": map[string]any{"nested": "whole aspect"}}) {
		t.Fatalf("same reference REMOVE lost siblings: %#v %v", value, err)
	}
	replayed, err := e.Execute(context.Background(), objectmodel.OpRemove, service.MaintenanceRequest{Reference: ref, Repository: s.RepositoryID, CommandID: "remove-member"})
	if err != nil || replayed.Result.NewCommit != removed.Result.NewCommit || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != removed.Result.NewCommit {
		t.Fatalf("same REMOVE must recover its original basis after the unit is gone: %#v %v", replayed, err)
	}
	_, err = e.Execute(context.Background(), objectmodel.OpPut, service.MaintenanceRequest{Reference: ref, Repository: s.RepositoryID, CommandID: "replace", Value: "different"})
	if kernel.CodeOf(err) != kernel.ErrIdempotencyConflict || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != removed.Result.NewCommit {
		t.Fatalf("same command with changed content must refuse without publication: %v", err)
	}
}

func TestReferenceMaintenanceRequiresAnExplicitNewMemberModel(t *testing.T) {
	s := testkit.NewSetup(t, "kr://object/new-member")
	e := service.MaintenanceExecutor{Writer: s.Writer, Repository: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return s.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}}
	ref, _ := objectmodel.FormatReference(objectmodel.Reference{Repository: s.RepositoryID, Object: "table/new", Locator: "/columns/id"})
	_, err := e.Execute(context.Background(), objectmodel.OpPut, service.MaintenanceRequest{Reference: ref, Repository: s.RepositoryID, CommandID: "ambiguous", Value: nil})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != s.RootCommitID {
		t.Fatalf("new collection model cannot be guessed from depth: %v", err)
	}
	receipt, err := e.Execute(context.Background(), objectmodel.OpPut, service.MaintenanceRequest{Reference: ref, Repository: s.RepositoryID, CommandID: "create-member", Kind: objectmodel.KindMember, Value: nil, IfAbsent: true})
	if err != nil {
		t.Fatal(err)
	}
	read, err := s.Reader.ReadAddress(s.RepositoryID, objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/new", AspectName: "columns", MemberKey: "id"}, receipt.Result.NewCommit)
	if err != nil || read.Value != nil {
		t.Fatalf("present null member must be readable: %#v %v", read, err)
	}
}

func TestReferenceMaintenanceExpectedAloneKeepsCASAndDoesNotPublish(t *testing.T) {
	fake := testkit.NewLakeFSFake(t)
	id := kernel.RepositoryID("kr://object/reference-cas")
	repo, err := snapshot.OpenLakeFS(id, fake.DSN(fake.NewRepo()), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	stores := snapshot.NewRegistry()
	if err := stores.Add(repo); err != nil {
		t.Fatal(err)
	}
	ledger, err := snapshot.NewCommandLedger(nil)
	if err != nil {
		t.Fatal(err)
	}
	writes, err := writer.NewWriter(stores, ledger)
	if err != nil {
		t.Fatal(err)
	}
	reads := reader.NewReader(stores)
	e := service.MaintenanceExecutor{Writer: writes, Repository: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return reads.Require(id, kernel.ErrObjectRefUnresolved)
	}}
	ref, err := objectmodel.FormatReference(objectmodel.Reference{Repository: id, Object: "note/cas"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := e.Execute(context.Background(), objectmodel.OpPut, service.MaintenanceRequest{Reference: ref, Repository: id, CommandID: "seed", Value: "original"})
	if err != nil {
		t.Fatal(err)
	}
	advanced, err := e.Execute(context.Background(), objectmodel.OpPut, service.MaintenanceRequest{Reference: ref, Repository: id, CommandID: "advance", Value: "kept"})
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []objectmodel.OpKind{objectmodel.OpPut, objectmodel.OpRemove} {
		t.Run(string(operation), func(t *testing.T) {
			_, err := e.Execute(context.Background(), operation, service.MaintenanceRequest{
				Reference: ref, Repository: id, CommandID: "stale-" + string(operation), Expected: first.Result.NewCommit, Value: "rejected",
			})
			if kernel.CodeOf(err) != kernel.ErrNonFastForward {
				t.Fatalf("stale expected commit must retain its CAS failure: %v", err)
			}
			if head := testkit.MustHead(t, repo, snapshot.DefaultRef); head != advanced.Result.NewCommit {
				t.Fatalf("rejected maintenance moved the published head: %s", head)
			}
			value, err := reads.ReadReference(objectmodel.Reference{Repository: id, Object: "note/cas"}, advanced.Result.NewCommit)
			if err != nil || value.Value != "kept" {
				t.Fatalf("rejected maintenance changed the published content: %#v %v", value.Value, err)
			}
		})
	}
}

func TestReferenceMaintenancePreservesExistingRelationKind(t *testing.T) {
	s := testkit.NewSetup(t, "kr://object/relation-kind")
	e := service.MaintenanceExecutor{Writer: s.Writer, Repository: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return s.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}}
	ref, _ := objectmodel.FormatReference(objectmodel.Reference{Repository: s.RepositoryID, Object: "relation/owns"})
	value := map[string]any{"relationId": "relation/owns", "relationType": "owns", "direction": "DIRECTED", "endpoints": []any{
		map[string]any{"role": "owner", "object": "team/a"},
		map[string]any{"role": "owned", "object": "table/orders"},
	}}
	_, err := e.Execute(context.Background(), objectmodel.OpPut, service.MaintenanceRequest{Reference: ref, Repository: s.RepositoryID, CommandID: "create-relation", Kind: objectmodel.KindRelation, Value: value})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := e.Execute(context.Background(), objectmodel.OpPut, service.MaintenanceRequest{Reference: ref, Repository: s.RepositoryID, CommandID: "replace-relation", Value: value})
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := s.Reader.Resolve(objectmodel.ObjectRef{Repository: s.RepositoryID, Object: "relation/owns"}, updated.Result.NewCommit)
	if err != nil || resolution.Address.Kind != objectmodel.KindRelation {
		t.Fatalf("root URI replaced the existing maintenance kind: %#v %v", resolution, err)
	}
	_, err = e.Execute(context.Background(), objectmodel.OpPut, service.MaintenanceRequest{Reference: ref, Repository: s.RepositoryID, CommandID: "invalid-relation", Value: map[string]any{"body": "not a relation"}})
	if err == nil || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != updated.Result.NewCommit {
		t.Fatalf("omitted kind bypassed Relation validation: %v", err)
	}
	_, err = e.Execute(context.Background(), objectmodel.OpPut, service.MaintenanceRequest{Reference: ref, Repository: s.RepositoryID, CommandID: "reinterpret-relation", Kind: objectmodel.KindEntity, Value: value})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != updated.Result.NewCommit {
		t.Fatalf("explicit kind reinterpreted an existing Relation: %v", err)
	}
}
