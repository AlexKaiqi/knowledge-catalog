package invocation

import (
	"context"
	"reflect"
	"testing"

	"kc/home"
	"kc/internal/testkit"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestBasicReferenceWritesRejectSeparateAddressesWithoutPublishing(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(*Invocation) (any, error)
	}{
		{"put", VerbPut}, {"remove", VerbRemove},
	} {
		for _, address := range []objectmodel.Address{
			{Kind: objectmodel.KindEntity, ObjectID: "note/one"},
			{Kind: objectmodel.KindAspect, ObjectID: "table/one", AspectName: "description"},
			{Kind: objectmodel.KindMember, ObjectID: "table/two", AspectName: "columns", MemberKey: "id"},
		} {
			for _, withReference := range []bool{false, true} {
				name := operation.name + "/" + string(address.Kind)
				if withReference {
					name += "/same-reference"
				}
				t.Run(name, func(t *testing.T) {
					s := testkit.NewSetup(t, "kr://invocation/single-reference")
					seed, err := s.Writer.Commit("seed", objectmodel.ChangeSet{
						TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
						BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
						Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: address, Value: "original"}},
					})
					if err != nil {
						t.Fatal(err)
					}
					flags := map[string]invoke.FlagValue{
						"repo": string(s.RepositoryID), "object": string(address.ObjectID),
						"command-id": "reject-separate", "value": `"changed"`,
					}
					if address.AspectName != "" {
						flags["aspect"] = address.AspectName
					}
					if address.MemberKey != "" {
						flags["member"] = address.MemberKey
					}
					if withReference {
						locator := ""
						if address.AspectName != "" {
							locator = "/" + address.AspectName
						}
						if address.MemberKey != "" {
							locator += "/" + address.MemberKey
						}
						flags["reference"], err = objectmodel.FormatReference(objectmodel.Reference{Repository: s.RepositoryID, Object: address.ObjectID, Locator: locator})
						if err != nil {
							t.Fatal(err)
						}
					}
					_, refused := operation.run(&Invocation{WS: &home.Home{Store: s.Store, Writer: s.Writer, Reader: s.Reader}, Flags: flags, Context: context.Background()})
					head := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
					value, readErr := s.Reader.ReadAddress(s.RepositoryID, address, head)
					if kernel.CodeOf(refused) != kernel.ErrUsageInvalid || head != seed.Result.NewCommit || readErr != nil || value.Value != "original" {
						t.Fatalf("separate addressing mutated content: error=%v head=%s seed=%s value=%#v read=%v", refused, head, seed.Result.NewCommit, value.Value, readErr)
					}
				})
			}
		}
	}
}

func TestBasicReferenceWritesPreserveSchemaCASAndOriginalReceipts(t *testing.T) {
	s := testkit.NewSetup(t, "kr://invocation/reference-write-contract")
	unit := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/one", AspectName: "structure"}
	seed, err := s.Writer.Commit("seed", objectmodel.ChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: map[string]any{
				"metaSchema": string(objectmodel.MetaSchemaV1), "entity": "Policy", "aspect": "structure", "pattern": "record", "additionalProperties": false,
				"fields": map[string]any{"name": map[string]any{"type": "string", "required": true}, "priority": map[string]any{"type": "integer"}},
			}},
			{Op: objectmodel.OpPut, Address: unit, SchemaRef: "schema/policy/structure/v1", Value: map[string]any{"name": "old", "priority": 1}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: unit.ObjectID, AspectName: "tag"}, Value: "kept"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.Reader.ReadAddress(s.RepositoryID, unit, seed.Result.NewCommit)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := objectmodel.FormatReference(objectmodel.Reference{Repository: s.RepositoryID, Object: unit.ObjectID, Locator: "/structure"})
	if err != nil {
		t.Fatal(err)
	}
	ws := &home.Home{Store: s.Store, Writer: s.Writer, Reader: s.Reader}
	call := func(run func(*Invocation) (any, error), command string, extra map[string]invoke.FlagValue) (objectmodel.CommitReceipt, error) {
		flags := map[string]invoke.FlagValue{"reference": ref, "repo": string(s.RepositoryID), "command-id": command}
		for key, value := range extra {
			flags[key] = value
		}
		result, err := run(&Invocation{WS: ws, Flags: flags, Context: context.Background()})
		if err != nil {
			return objectmodel.CommitReceipt{}, err
		}
		receipt, ok := result.(objectmodel.CommitReceipt)
		if !ok {
			t.Fatalf("maintenance did not return the original Writer receipt: %T", result)
		}
		return receipt, nil
	}
	put := map[string]invoke.FlagValue{"base": string(seed.Result.NewCommit), "expected": string(seed.Result.NewCommit), "if-digest": string(before.Digest), "value": `{"name":"new"}`}
	updated, err := call(VerbPut, "replace", put)
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.Reader.Read(objectmodel.ObjectRef{Repository: s.RepositoryID, Object: unit.ObjectID}, updated.Result.NewCommit, nil)
	want := map[string]any{"structure": map[string]any{"name": "new"}, "tag": "kept"}
	if err != nil || !reflect.DeepEqual(current.Value, want) {
		t.Fatalf("Reference PUT merged old fields or replaced siblings: %#v %v", current.Value, err)
	}
	replayed, err := call(VerbPut, "replace", put)
	if err != nil || replayed.Disposition != objectmodel.DispositionReplayed || replayed.Result != updated.Result || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != updated.Result.NewCommit {
		t.Fatalf("Reference PUT did not recover its original receipt: %#v %v", replayed, err)
	}
	for _, refusal := range []struct {
		command string
		flags   map[string]invoke.FlagValue
		code    kernel.ErrorCode
	}{
		{"stale-cas", put, kernel.ErrNonFastForward},
		{"stale-digest", map[string]invoke.FlagValue{"base": string(updated.Result.NewCommit), "if-digest": string(before.Digest), "value": `{"name":"other"}`}, kernel.ErrPreconditionFailed},
		{"invalid-schema", map[string]invoke.FlagValue{"value": `{"name":123}`}, kernel.ErrSchemaInstanceInvalid},
		{"replace", map[string]invoke.FlagValue{"base": string(seed.Result.NewCommit), "expected": string(seed.Result.NewCommit), "if-digest": string(before.Digest), "value": `{"name":"different"}`}, kernel.ErrIdempotencyConflict},
	} {
		_, err := call(VerbPut, refusal.command, refusal.flags)
		at := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
		value, readErr := s.Reader.Read(objectmodel.ObjectRef{Repository: s.RepositoryID, Object: unit.ObjectID}, at, nil)
		if kernel.CodeOf(err) != refusal.code || at != updated.Result.NewCommit || readErr != nil || !reflect.DeepEqual(value.Value, want) {
			t.Fatalf("%s changed accepted content: err=%v head=%s value=%#v read=%v", refusal.command, err, at, value.Value, readErr)
		}
	}
	selected, err := s.Reader.ReadAddress(s.RepositoryID, unit, updated.Result.NewCommit)
	if err != nil {
		t.Fatal(err)
	}
	remove := map[string]invoke.FlagValue{"base": string(updated.Result.NewCommit), "if-digest": string(selected.Digest)}
	removed, err := call(VerbRemove, "remove", remove)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err = call(VerbRemove, "remove", remove)
	if err != nil || replayed.Disposition != objectmodel.DispositionReplayed || replayed.Result != removed.Result || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != removed.Result.NewCommit {
		t.Fatalf("Reference REMOVE did not recover after deletion: %#v %v", replayed, err)
	}
	remaining, err := s.Reader.Read(objectmodel.ObjectRef{Repository: s.RepositoryID, Object: unit.ObjectID}, removed.Result.NewCommit, nil)
	if err != nil || !reflect.DeepEqual(remaining.Value, map[string]any{"tag": "kept"}) {
		t.Fatalf("Reference REMOVE changed a sibling: %#v %v", remaining.Value, err)
	}
}
