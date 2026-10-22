package reader_test

import (
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

func TestDatasetDependencyBasisRetainsNonContributingCandidates(t *testing.T) {
	source := testkit.NewSetup(t, "kr://basis/source")
	descriptors := testkit.NewSetup(t, "kr://basis/descriptors")
	absent := testkit.NewSetup(t, "kr://basis/absent")
	const descriptor objectmodel.ObjectID = "resource/orders"
	original := bindingfixture.Descriptor("https://old.example")
	first, err := descriptors.Writer.Commit("dependency-descriptor", objectmodel.ChangeSet{TargetRepository: descriptors.RepositoryID,
		TargetRef: snapshot.DefaultRef, BaseCommit: descriptors.RootCommitID, ExpectedTargetCommit: descriptors.RootCommitID,
		Operations: []objectmodel.Operation{bindingfixture.PutDescriptor(descriptor, original)}})
	if err != nil {
		t.Fatal(err)
	}
	registry := snapshot.NewRegistry()
	for _, repo := range []snapshot.Store{source.Repo, descriptors.Repo, absent.Repo} {
		if err := registry.Add(repo); err != nil {
			t.Fatal(err)
		}
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := w.Commit("dependency-source", objectmodel.ChangeSet{TargetRepository: source.RepositoryID,
		TargetRef: snapshot.DefaultRef, BaseCommit: source.RootCommitID, ExpectedTargetCommit: source.RootCommitID,
		Operations: []objectmodel.Operation{bindingfixture.Bind(objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "runtime"}, "", descriptor)}})
	if err != nil {
		t.Fatal(err)
	}
	rd := reader.NewReader(registry)
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return rd.Require(id, kernel.ErrObjectRefUnresolved)
	}
	versions := map[kernel.RepositoryID]kernel.CommitID{source.RepositoryID: bound.Result.NewCommit}
	pin := reader.DatasetPin{SetID: "orders", Revision: 1, Repositories: versions, Items: reader.WholeRepositoryItems(versions)}
	dependencies, err := reader.Open(lookup, pin).FreezeDependencies(registry.IDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(dependencies) != 3 || dependencies[source.RepositoryID] != bound.Result.NewCommit || dependencies[descriptors.RepositoryID] != first.Result.NewCommit || dependencies[absent.RepositoryID] != absent.RootCommitID {
		t.Fatalf("interpretation vector lost candidate absence: %#v", dependencies)
	}
	_, err = absent.Writer.Commit("dependency-new-holder", objectmodel.ChangeSet{TargetRepository: absent.RepositoryID,
		TargetRef: snapshot.DefaultRef, BaseCommit: absent.RootCommitID, ExpectedTargetCommit: absent.RootCommitID,
		Operations: []objectmodel.Operation{bindingfixture.PutDescriptor(descriptor, bindingfixture.Descriptor("https://new.example"))}})
	if err != nil {
		t.Fatal(err)
	}
	pin.Dependencies = dependencies
	value, err := reader.Open(lookup, pin).ReadDescriptor(descriptor)
	if err != nil || value.Commit != first.Result.NewCommit || kernel.CanonicalDigest(value.Value) != kernel.CanonicalDigest(original) {
		t.Fatalf("frozen interpretation followed a new candidate: %#v %v", value, err)
	}
}
