package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type malformedReadHydrator struct {
	mutate func(*objectmodel.ObjectValue)
}

func (h malformedReadHydrator) ReadMany(repo objectmodel.Repository, commit kernel.CommitID, ids []objectmodel.ObjectID) (map[objectmodel.ObjectID]objectmodel.ObjectValue, error) {
	values := make(map[objectmodel.ObjectID]objectmodel.ObjectValue, len(ids))
	for _, id := range ids {
		value, err := repo.Read(id, commit)
		if err != nil {
			return nil, err
		}
		h.mutate(&value)
		values[id] = value
	}
	return values, nil
}

func (h malformedReadHydrator) ReadAddress(repo objectmodel.Repository, commit kernel.CommitID, address objectmodel.Address) (objectmodel.ObjectValue, error) {
	value, err := repo.ReadAddress(address, commit)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	h.mutate(&value)
	return value, nil
}

func TestInjectedHydrationRejectsMalformedReaderAndServingValues(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/hydration-validation")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/a", AspectName: "body"}
	commit, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: address, Value: map[string]any{"text": "original"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := snapshot.NewRegistry()
	if err := registry.Add(repo); err != nil {
		t.Fatal(err)
	}
	ref := objectmodel.ObjectRef{Repository: repo.ID(), Object: address.ObjectID}
	cases := []struct {
		name       string
		objectOnly bool
		mutate     func(*objectmodel.ObjectValue)
	}{
		{"repository", false, func(v *objectmodel.ObjectValue) { v.Repository = "kr://other" }},
		{"ref-repository", false, func(v *objectmodel.ObjectValue) { v.ObjectRef.Repository = "kr://other" }},
		{"ref-object", false, func(v *objectmodel.ObjectValue) { v.ObjectRef.Object = "policy/other" }},
		{"commit", false, func(v *objectmodel.ObjectValue) { v.Commit = "other-commit" }},
		{"object", false, func(v *objectmodel.ObjectValue) { v.Address.ObjectID = "policy/other" }},
		{"aspect-as-object", true, func(v *objectmodel.ObjectValue) {
			v.Address = address
			v.Value = map[string]any{"text": "only one aspect"}
		}},
		{"foreign-unit", false, func(v *objectmodel.ObjectValue) {
			v.Units = []objectmodel.Address{{Kind: objectmodel.KindAspect, ObjectID: "policy/other", AspectName: "body"}}
		}},
		{"foreign-declaration", false, func(v *objectmodel.ObjectValue) {
			v.Declarations = []objectmodel.UnitDeclaration{{Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/other", AspectName: "body"}}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rd := reader.NewReader(registry)
			hydrator := malformedReadHydrator{mutate: tc.mutate}
			rd.SetHydrator(hydrator)
			serving := reader.Open(rd.Lookup(func(id kernel.RepositoryID) (snapshot.Store, error) {
				return registry.Require(id, kernel.ErrObjectRefUnresolved)
			}), reader.DatasetPin{SetID: "fixed", Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit}, Items: reader.WholeRepositoryItems(map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit})})
			serving.SetHydrator(hydrator)
			checks := map[string]func() error{
				"Reader.Read":  func() error { _, err := rd.Read(ref, commit, nil); return err },
				"Serving.Read": func() error { _, err := serving.Read(ref.Object, nil); return err },
			}
			if !tc.objectOnly {
				checks["Reader.ReadAddress"] = func() error { _, err := rd.ReadAddress(repo.ID(), address, commit); return err }
				checks["Serving.ReadAddress"] = func() error { _, err := serving.ReadAddress(address); return err }
			}
			for name, read := range checks {
				t.Run(name, func(t *testing.T) {
					if err := read(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
						t.Fatalf("malformed hydration was accepted: %v", err)
					}
				})
			}
		})
	}
	// Kind is metadata of the Canonical unit; matching continues to use the
	// object/aspect/member coordinates rather than imposing a new Kind rule.
	rd := reader.NewReader(registry)
	rd.SetHydrator(malformedReadHydrator{mutate: func(v *objectmodel.ObjectValue) { v.Address.Kind = objectmodel.KindRecord }})
	if _, err := rd.ReadAddress(repo.ID(), address, commit); err != nil {
		t.Fatalf("canonical Kind variation was rejected: %v", err)
	}
}
