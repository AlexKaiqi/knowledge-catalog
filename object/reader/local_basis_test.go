package reader_test

import (
	"context"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

func TestLocalReadBasisKeepsUnavailableInterpretationCoordinates(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	base, err := rd.Wrap(&basisSnapshot{referenceReadGuard: guard, binding: "local-source"}, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	source := base
	const dependency kernel.RepositoryID = "kr://comparison/dependency"
	dependencyRepo := &basisRepository{Repository: basisRenamedRepository{Repository: base, id: dependency}, binding: "dependency", missing: at}
	available := true
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id == source.ID() {
			return source, nil
		}
		if id != dependency {
			t.Fatalf("invented dependency: %s", id)
		}
		if !available {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "dependency offline")
		}
		return dependencyRepo, nil
	}
	pin := reader.DatasetPin{SetID: "published", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{source.ID(): at}, Dependencies: map[kernel.RepositoryID]kernel.CommitID{dependency: at}, Items: []reader.DatasetItem{{Repository: source.ID(), Commit: at, Kind: "file", File: "public/id.yaml"}}}
	serving := reader.Open(lookup, pin)
	basis, err := serving.LocalReadBasis(objectmodel.Reference{Object: "table/orders"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(basis.Dependencies) != 1 || basis.Dependencies[0].Commit != at || basis.Dependencies[0].Binding != "dependency" {
		t.Fatalf("missing commit lost fixed coordinates: %#v", basis)
	}
	if _, err := reader.OpenReadBasis(lookup, basis); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("unavailable interpretation replayed: %v", err)
	}
	first, err := reader.ReadComparisonBasis(context.Background(), lookup, basis)
	if err != nil {
		t.Fatal(err)
	}
	available = false
	offline, err := serving.LocalReadBasis(objectmodel.Reference{Object: "table/orders"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(offline.UnresolvedDependencies) != 1 || offline.UnresolvedDependencies[0].Repository != dependency || offline.UnresolvedDependencies[0].Commit != at {
		t.Fatalf("offline coordinate lost: %#v", offline)
	}
	second, err := reader.ReadComparisonBasis(context.Background(), lookup, offline)
	if err != nil {
		t.Fatal(err)
	}
	compared, err := objectmodel.CompareLocalReads(first, second)
	if err != nil || compared.Equal == nil || !*compared.Equal {
		t.Fatalf("interpretation outage changed local content: %#v %v", compared, err)
	}
}
