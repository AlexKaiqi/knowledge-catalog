package reader_test

import (
	"reflect"
	"testing"

	"kc/internal/bindingfixture"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/object/reader"
	"kc/snapshot"
)

// The same Schema can describe both stored and external member values. Only
// the actual selected binding units may affect planning or freeze dependencies.
func TestDatasetBindingMetadataFollowsSelectedUnits(t *testing.T) {
	rd, guard, w, base := newReferenceReadFixture(t)
	const schemaID objectmodel.ObjectID = "schema/sample"
	const descriptorID objectmodel.ObjectID = "resource/samples"
	const schemaPath = "schemas/sample.yaml"
	const descriptorPath = "private/descriptor.yaml"
	static := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "metric/static", AspectName: "samples", MemberKey: "one"}
	hidden := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "metric/hidden", AspectName: "samples", MemberKey: "one"}
	mixedStatic := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "samples", MemberKey: "stored"}
	mixedBound := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "samples", MemberKey: "external"}
	stored := func(address objectmodel.Address, path string) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: address, SchemaRef: string(schemaID), PathHint: path, Value: map[string]any{"value": "fixed"}}
	}
	receipt, err := w.Commit("publish-scoped-bindings", objectmodel.ChangeSet{
		TargetRepository: guard.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: schemaID}, PathHint: schemaPath,
				Value: map[string]any{"entity": "Sample", "aspect": "samples", "pattern": "keyed_collection", "fields": map[string]any{"value": map[string]any{"type": "string"}}}},
			bindingfixture.WithPath(bindingfixture.PutDescriptor(descriptorID, bindingfixture.Descriptor("https://samples.example")), descriptorPath),
			stored(static, "public/static.yaml"),
			stored(mixedStatic, "public/mixed.yaml"),
			bindingfixture.WithPath(bindingfixture.Bind(hidden, string(schemaID), descriptorID), "private/hidden.yaml"),
			bindingfixture.WithPath(bindingfixture.Bind(mixedBound, string(schemaID), descriptorID), "private/mixed.yaml"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	at := receipt.Result.NewCommit
	for _, path := range []string{schemaPath, descriptorPath, "public/static.yaml", "public/mixed.yaml", "private/hidden.yaml", "private/mixed.yaml", "public/id.yaml", "private/description.yaml", "private/status.yaml"} {
		guard.blocked[path] = true
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return rd.Require(id, kernel.ErrObjectRefUnresolved)
	}
	open := func(paths ...string) *reader.Serving {
		items := []reader.DatasetItem{{Repository: guard.ID(), Commit: at, Kind: "file", File: schemaPath, Target: schemaPath}}
		for _, path := range paths {
			items = append(items, reader.DatasetItem{Repository: guard.ID(), Commit: at, Kind: "file", File: path, Target: path})
		}
		return reader.Open(lookup, reader.DatasetPin{SetID: "samples", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{guard.ID(): at}, Items: items})
	}
	for _, test := range []struct {
		name  string
		paths []string
		bound bool
	}{
		{"only Schema and unselected bindings", nil, false},
		{"stored object with an unselected bound object", []string{"public/static.yaml"}, false},
		{"stored member with an unselected bound sibling", []string{"public/mixed.yaml"}, false},
		{"selected bound object", []string{"private/hidden.yaml"}, true},
		{"selected bound member without its stored sibling", []string{"private/mixed.yaml"}, true},
		{"stored and bound members", []string{"public/mixed.yaml", "private/mixed.yaml"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			guard.reads = map[string]int{}
			guard.blocked[descriptorPath] = true
			serving := open(test.paths...)
			member, err := serving.Member(guard.ID())
			if err != nil {
				t.Fatal(err)
			}
			ids, err := member.(objectmodel.BindingLocator).BindingSchemaObjectIDs(at)
			if err != nil || test.bound && !reflect.DeepEqual(ids, []objectmodel.ObjectID{schemaID}) || !test.bound && len(ids) != 0 {
				t.Fatalf("binding Schema classification escaped selected units: ids=%v wantBound=%v error=%v", ids, test.bound, err)
			}
			assertBindingScopeBodiesUnread(t, guard)
			// Dependency freezing may read the named descriptor, but must not
			// inspect any selected or unselected binding unit body to find it.
			guard.blocked[descriptorPath] = false
			dependencies, err := serving.FreezeDependencies(func() []kernel.RepositoryID { return []kernel.RepositoryID{guard.ID()} })
			var want map[kernel.RepositoryID]kernel.CommitID
			if test.bound {
				want = map[kernel.RepositoryID]kernel.CommitID{guard.ID(): at}
			}
			if err != nil || !reflect.DeepEqual(dependencies, want) {
				t.Fatalf("descriptor dependencies did not follow selected units: got=%v want=%v error=%v", dependencies, want, err)
			}
			if !test.bound && guard.reads[descriptorPath] != 0 {
				t.Fatal("an unselected binding triggered descriptor dependency reads")
			}
			assertBindingScopeBodiesUnread(t, guard)
		})
	}
	t.Run("missing maintenance coordinates fail closed", func(t *testing.T) {
		guard.reads = map[string]int{}
		guard.blocked[descriptorPath] = true
		paths, err := repofile.ReadObjectLocator(guard.TreeStore, mixedBound.ObjectID, at)
		if err != nil {
			t.Fatal(err)
		}
		guard.locatorOverride, err = repofile.EncodeObjectLocator(mixedBound.ObjectID, paths)
		if err != nil {
			t.Fatal(err)
		}
		complete, err := open(paths...).Member(guard.ID())
		if err != nil {
			t.Fatal(err)
		}
		if ids, err := complete.(objectmodel.BindingLocator).BindingSchemaObjectIDs(at); err != nil || !reflect.DeepEqual(ids, []objectmodel.ObjectID{schemaID}) {
			t.Fatalf("complete-object paths-only metadata stopped proving binding scope: ids=%v error=%v", ids, err)
		}
		member, err := open("private/mixed.yaml").Member(guard.ID())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.(objectmodel.BindingLocator).BindingSchemaObjectIDs(at); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
			t.Fatalf("missing coordinates guessed binding scope: %v", err)
		}
		assertBindingScopeBodiesUnread(t, guard)
	})
}

func assertBindingScopeBodiesUnread(t *testing.T, guard *referenceReadGuard) {
	t.Helper()
	for path, blocked := range guard.blocked {
		if blocked && guard.reads[path] != 0 {
			t.Fatalf("binding scope metadata accessed unit body %s", path)
		}
	}
}
