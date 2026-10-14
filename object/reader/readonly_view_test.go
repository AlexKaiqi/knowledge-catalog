package reader_test

import (
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type readonlyRefCreator interface {
	CreateRef(string, kernel.CommitID) error
}

type readonlyRefMerger interface {
	Merge(string, kernel.CommitID, kernel.CommitID) (kernel.CommitID, error)
}

type readonlyArchiver interface {
	Archive() error
}

func TestReaderReturnsActualReadOnlyRepositoryViews(t *testing.T) {
	providers := []struct {
		name string
		open func(*testing.T) (snapshot.Store, objectmodel.ObjectID)
	}{
		{
			name: "tree",
			open: func(t *testing.T) (snapshot.Store, objectmodel.ObjectID) {
				repo := testkit.MakeRepository(t, "kr://regression/readonly")
				base := testkit.MustHead(t, repo, snapshot.DefaultRef)
				const objectID objectmodel.ObjectID = "readonly/probe"
				_, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{
					TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
					BaseCommit: base, ExpectedTargetCommit: base,
					Operations: testkit.PutEntity(string(objectID), map[string]any{"value": "fixed"}, "probe.json"),
				})
				if err != nil {
					t.Fatal(err)
				}
				return repo.Snapshot(), objectID
			},
		},
		{
			name: "native",
			open: func(t *testing.T) (snapshot.Store, objectmodel.ObjectID) {
				return testkit.MakeNativeSystemRepository(t), objectmodel.SystemReadmeObjectID
			},
		},
	}
	for _, provider := range providers {
		for _, entrypoint := range []string{"Require", "Wrap", "Member"} {
			t.Run(provider.name+"/"+entrypoint, func(t *testing.T) {
				raw, objectID := provider.open(t)
				commit := testkit.MustHead(t, raw, snapshot.DefaultRef)
				var wantValue any = map[string]any{"value": "fixed"}
				if native, ok := raw.(objectmodel.NativeRepository); ok {
					original, err := native.Read(objectID, commit)
					if err != nil {
						t.Fatal(err)
					}
					wantValue = original.Value
				}
				var view objectmodel.Repository
				var err error
				if entrypoint == "Require" || entrypoint == "Member" {
					registry := snapshot.NewRegistry()
					if err := registry.Add(raw); err != nil {
						t.Fatal(err)
					}
					view, err = reader.NewReader(registry).Require(raw.ID(), kernel.ErrCapabilityUnsatisfied)
				} else {
					view, err = reader.NewReader(nil).Wrap(raw, kernel.ErrCapabilityUnsatisfied)
				}
				if err != nil {
					t.Fatal(err)
				}
				if entrypoint == "Member" {
					baseView := view
					serving := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return baseView, nil }, reader.DatasetPin{
						Repositories: map[kernel.RepositoryID]kernel.CommitID{raw.ID(): commit},
						Items:        reader.WholeRepositoryItems(map[kernel.RepositoryID]kernel.CommitID{raw.ID(): commit}),
					})
					view, err = serving.Member(raw.ID())
					if err != nil {
						t.Fatal(err)
					}
				}
				if view == nil {
					t.Fatal("read-only view is nil")
				}
				if view.ID() != raw.ID() {
					t.Errorf("read-only view changed repository identity: got=%s want=%s", view.ID(), raw.ID())
				}
				value, err := view.Read(objectID, commit)
				if err != nil {
					t.Errorf("read-only view lost fixed read capability: %v", err)
				} else if value.Repository != raw.ID() || value.Commit != commit || value.ObjectRef.Repository != raw.ID() || value.ObjectRef.Object != objectID || value.Address.ObjectID != objectID {
					t.Errorf("read-only view changed fixed read coordinates: %#v", value)
				}
				if err == nil && !reflect.DeepEqual(value.Value, wantValue) {
					t.Errorf("read-only view changed fixed object value: got=%#v want=%#v", value.Value, wantValue)
				}

				assertNoExportedWriteProvider(t, reflect.ValueOf(view), 0)

				// Check the actual value's independent capabilities. A partial write
				// leak must not escape detection just because Store no longer matches.
				if _, ok := view.(readonlyRefCreator); ok {
					t.Errorf("read-only %s view still carries CreateRef capability: %T", entrypoint, view)
				}
				if _, ok := view.(readonlyRefMerger); ok {
					t.Errorf("read-only %s view still carries Merge capability: %T", entrypoint, view)
				}
				if _, ok := view.(readonlyArchiver); ok {
					t.Errorf("read-only %s view still carries Archive capability: %T", entrypoint, view)
				}
			})
		}
	}
}

func assertNoExportedWriteProvider(t *testing.T, value reflect.Value, depth int) {
	t.Helper()
	if !value.IsValid() || depth > 4 {
		return
	}
	if value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return
		}
		assertNoExportedWriteProvider(t, value.Elem(), depth+1)
		return
	}
	if value.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if !field.CanInterface() {
			continue
		}
		name := value.Type().Field(i).Name
		if _, ok := field.Interface().(readonlyRefCreator); ok {
			t.Errorf("read-only view exposes writable provider through exported field %s", name)
		}
		if _, ok := field.Interface().(readonlyRefMerger); ok {
			t.Errorf("read-only view exposes Merge through exported field %s", name)
		}
		if _, ok := field.Interface().(readonlyArchiver); ok {
			t.Errorf("read-only view exposes Archive through exported field %s", name)
		}
		assertNoExportedWriteProvider(t, field, depth+1)
	}
}
