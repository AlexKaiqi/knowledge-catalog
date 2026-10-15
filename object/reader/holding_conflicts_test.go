package reader_test

import (
	"strings"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type conflictSource struct {
	setup  testkit.Setup
	commit kernel.CommitID
}

func newConflictSource(t *testing.T, id string, aspects map[string]string) conflictSource {
	t.Helper()
	s := testkit.NewSetup(t, id)
	ops := []objectmodel.Operation{}
	for aspect, path := range aspects {
		ops = append(ops, objectmodel.Operation{Op: objectmodel.OpPut, PathHint: path,
			Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: aspect}, Value: map[string]any{"from": id}})
	}
	commit, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: ops})
	if err != nil {
		t.Fatal(err)
	}
	return conflictSource{setup: s, commit: commit}
}

func conflictServing(t *testing.T, items []reader.DatasetItem, sources ...conflictSource) *reader.Serving {
	t.Helper()
	repos := map[kernel.RepositoryID]conflictSource{}
	commits := map[kernel.RepositoryID]kernel.CommitID{}
	for _, source := range sources {
		repos[source.setup.RepositoryID] = source
		commits[source.setup.RepositoryID] = source.commit
	}
	if items == nil {
		items = reader.WholeRepositoryItems(commits)
	}
	return reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return repos[id].setup.Reader.Require(id, kernel.ErrCapabilityUnsatisfied)
	}, reader.DatasetPin{SetID: "conflict", Revision: 1, Repositories: commits, Items: items})
}

// Dataset publication rejects the same Aspect of one object from two source
// Repositories and names the object, Aspect and sources (doc/design/dataset.md).
func TestPublicationRejectsSameAspectFromTwoSources(t *testing.T) {
	a := newConflictSource(t, "kr://conflict/a", map[string]string{"structure": "a/structure.yaml"})
	b := newConflictSource(t, "kr://conflict/b", map[string]string{"structure": "b/structure.yaml"})
	err := conflictServing(t, nil, a, b).CheckHoldingConflicts()
	if kernel.CodeOf(err) != kernel.ErrObjectIDConflict {
		t.Fatalf("same Aspect from two sources must be OBJECT_ID_CONFLICT, got %v", err)
	}
	for _, want := range []string{"table/orders", "structure", "kr://conflict/a", "kr://conflict/b"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("conflict must name %s: %v", want, err)
		}
	}
}

func TestPublicationComposesDisjointAspectsAndHonorsRange(t *testing.T) {
	a := newConflictSource(t, "kr://conflict/a", map[string]string{"structure": "a/structure.yaml"})
	b := newConflictSource(t, "kr://conflict/b", map[string]string{"ownership": "b/ownership.yaml"})
	if err := conflictServing(t, nil, a, b).CheckHoldingConflicts(); err != nil {
		t.Fatalf("disjoint Aspects compose into one object: %v", err)
	}
	c := newConflictSource(t, "kr://conflict/c", map[string]string{"structure": "private/structure.yaml"})
	items := []reader.DatasetItem{
		{Target: "a", Repository: a.setup.RepositoryID, Commit: a.commit, Kind: "prefix"},
		{Target: "c", Repository: c.setup.RepositoryID, Commit: c.commit, Kind: "prefix", Prefix: "public"},
	}
	if err := conflictServing(t, items, a, c).CheckHoldingConflicts(); err != nil {
		t.Fatalf("a holding outside the published range cannot conflict: %v", err)
	}
	d := testkit.NewSetup(t, "kr://conflict/d")
	dCommit, err := d.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: d.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: d.RootCommitID, ExpectedTargetCommit: d.RootCommitID, Operations: []objectmodel.Operation{
			bindingfixture.PutDescriptor("resource/structure", bindingfixture.Descriptor("https://structure.example")),
			bindingfixture.PutAspect("table/orders", "structure", "", "resource/structure"),
		}})
	if err != nil {
		t.Fatal(err)
	}
	if err := conflictServing(t, nil, a, conflictSource{setup: d, commit: dCommit}).CheckHoldingConflicts(); kernel.CodeOf(err) != kernel.ErrObjectIDConflict {
		t.Fatalf("a binding Aspect unit counts toward its Repository's holding, got %v", err)
	}
}

func TestPublicationConflictCheckSkipsSingleSource(t *testing.T) {
	a := newConflictSource(t, "kr://conflict/a", map[string]string{"structure": "a/structure.yaml"})
	serving := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) {
		t.Fatal("a single-source pin must not scan its source")
		return nil, nil
	}, reader.DatasetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{a.setup.RepositoryID: a.commit}})
	if err := serving.CheckHoldingConflicts(); err != nil {
		t.Fatal(err)
	}
}
