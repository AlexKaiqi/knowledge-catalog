package reader_test

import (
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// A single-repository commit fixes only what that repository stores. When an
// interpretation dependency advances and the primary commit does not, an
// independent request must adopt the new dependency version, while a replay
// that carries the original basis keeps the frozen coordinates. Both promises
// are asserted against the same fixture so neither can be met by weakening the
// other.
func TestFixedBasisKeepsDependencyCoordinatesWhileNewRequestAdvances(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	source, err := rd.Wrap(&basisSnapshot{referenceReadGuard: guard, binding: "primary-authority"}, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	const dependency kernel.RepositoryID = "kr://basis/descriptor"
	// missing names the commit the advanced dependency no longer holds, so the
	// frozen commit stays resolvable while the newer one is genuinely absent.
	dependencyFor := func(missing kernel.CommitID) *basisRepository {
		return &basisRepository{
			Repository: basisRenamedRepository{Repository: source, id: dependency},
			binding:    "descriptor-authority",
			missing:    missing,
		}
	}
	lookupFor := func(missing kernel.CommitID) func(kernel.RepositoryID) (objectmodel.Repository, error) {
		repo := dependencyFor(missing)
		return func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			switch id {
			case source.ID():
				return source, nil
			case dependency:
				return repo, nil
			}
			return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "unknown repository %s", id)
		}
	}
	items := []reader.DatasetItem{{Repository: source.ID(), Commit: at, Kind: "file", File: "public/id.yaml"}}
	pinned := func(dependencyCommit kernel.CommitID) reader.DatasetPin {
		return reader.DatasetPin{
			SetID: "published", Revision: 1,
			Repositories: map[kernel.RepositoryID]kernel.CommitID{source.ID(): at},
			Dependencies: map[kernel.RepositoryID]kernel.CommitID{dependency: dependencyCommit},
			Items:        items,
		}
	}

	const advanced kernel.CommitID = "advanced-descriptor-commit"

	basis, err := reader.Open(lookupFor(advanced), pinned(at)).ReadBasis(objectmodel.Reference{Object: "table/orders"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(basis.Dependencies) != 1 || basis.Dependencies[0].Repository != dependency || basis.Dependencies[0].Commit != at {
		t.Fatalf("basis did not freeze the interpretation dependency: %#v", basis)
	}

	// The dependency advances; the primary commit stays exactly where it was.
	reopened, err := reader.OpenReadBasis(lookupFor(advanced), basis)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Pin().Dependencies[dependency]; got != at {
		t.Fatalf("fixed replay followed the advanced dependency: %s", got)
	}
	if got := reopened.Pin().Repositories[source.ID()]; got != at {
		t.Fatalf("fixed replay moved the primary commit: %s", got)
	}

	// A new request over the same primary commit resolves the dependency as it
	// stands now, which is what the origin-coordinate promise allows.
	fresh := reader.Open(lookupFor(advanced), pinned(advanced))
	if got := fresh.Pin().Dependencies[dependency]; got != advanced {
		t.Fatalf("independent request reused an older dependency: %s", got)
	}
	if got := fresh.Pin().Repositories[source.ID()]; got != at {
		t.Fatalf("independent request moved the primary commit: %s", got)
	}
}
