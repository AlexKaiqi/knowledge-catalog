package reader_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type basisRepository struct {
	objectmodel.Repository
	binding kernel.Digest
	missing kernel.CommitID
}

func (r *basisRepository) StoreDigest() kernel.Digest { return r.binding }
func (*basisRepository) Head(string) (kernel.CommitID, error) {
	return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "fixed replay must not resolve a head")
}
func (r *basisRepository) HasCommit(commit kernel.CommitID) bool {
	return commit != r.missing && r.Repository.HasCommit(commit)
}
func (r *basisRepository) PlanReference(ref objectmodel.Reference, commit kernel.CommitID) ([]objectmodel.Address, error) {
	return r.Repository.(objectmodel.ReferencePlanner).PlanReference(ref, commit)
}
func (r *basisRepository) ReadReference(ref objectmodel.Reference, commit kernel.CommitID) (objectmodel.ObjectValue, error) {
	return r.Repository.(objectmodel.ReferenceReader).ReadReference(ref, commit)
}

type basisSnapshot struct {
	*referenceReadGuard
	binding string
	missing kernel.CommitID
}

func (r *basisSnapshot) Origin() string { return r.binding }
func (r *basisSnapshot) HasCommit(commit kernel.CommitID) bool {
	return commit != r.missing && r.referenceReadGuard.HasCommit(commit)
}
func (*basisSnapshot) Head(string) (kernel.CommitID, error) {
	return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "fixed replay must not resolve a head")
}

func TestReadBasisRoundTripKeepsCandidatesScopeAndOldCommit(t *testing.T) {
	_, guard, w, at := newReferenceReadFixture(t)
	bound := &basisSnapshot{referenceReadGuard: guard, binding: "authority-a"}
	repo, err := reader.NewReader(nil).Wrap(bound, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id != bound.ID() {
			return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "unknown repository")
		}
		return repo, nil
	}
	pin := reader.DatasetPin{SetID: "orders", Revision: 7, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at},
		Items: []reader.DatasetItem{{Repository: repo.ID(), Commit: at, Kind: "file", File: "public/id.yaml"}}}
	scope := reader.Open(lookup, pin)
	ref := objectmodel.Reference{Object: "table/orders", Locator: "/columns/order_id/constraints/nullable"}
	basis, err := scope.ReadBasis(ref, "")
	if err != nil {
		t.Fatal(err)
	}
	// Only portable data crosses the restart boundary. A new Serving and
	// resolver must reproduce the old selection after the source advances.
	raw, err := json.Marshal(basis)
	if err != nil {
		t.Fatal(err)
	}
	var restored objectmodel.ReadBasis
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("advance-after-basis", objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: at, ExpectedTargetCommit: at, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: ref.Object, AspectName: "columns", MemberKey: "order_id"},
			Value:   map[string]any{"constraints": map[string]any{"nullable": true}}}}})
	if err != nil {
		t.Fatal(err)
	}
	guard.blocked["private/description.yaml"], guard.blocked["private/status.yaml"] = true, true
	reopened, err := reader.OpenReadBasis(lookup, restored)
	if err != nil {
		t.Fatal(err)
	}
	readRef := ref
	readRef.Repository = repo.ID()
	values, err := reopened.ReadReference(readRef)
	if err != nil || len(values) != 1 || values[0].Value != false || values[0].Commit != at {
		t.Fatalf("fixed read drifted: %#v %v", values, err)
	}
	if !reflect.DeepEqual(reopened.Pin().Repositories, pin.Repositories) || reopened.Pin().Revision != 7 {
		t.Fatal("reopened different Dataset coordinates")
	}
	if _, err := reopened.ReadReference(objectmodel.Reference{Repository: repo.ID(), Object: ref.Object}); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("basis expanded a file slice to a whole object: %v", err)
	}
	for name, tc := range map[string]struct {
		mutate func()
		code   kernel.ErrorCode
	}{
		"replacement binding":  {func() { bound.binding = "other-authority" }, kernel.ErrPreconditionFailed},
		"missing fixed commit": {func() { bound.missing = at }, kernel.ErrVersionUnresolved},
	} {
		t.Run(name, func(t *testing.T) {
			bound.binding, bound.missing = "authority-a", ""
			tc.mutate()
			if _, err := reader.OpenReadBasis(lookup, restored); kernel.CodeOf(err) != tc.code {
				t.Fatalf("invalid source returned %v, want %s", err, tc.code)
			}
		})
	}
	if _, err := reader.OpenReadBasis(func(kernel.RepositoryID) (objectmodel.Repository, error) {
		return nil, kernel.Fail(kernel.ErrForbidden, "access revoked")
	}, restored); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("revocation lost: %v", err)
	}
}

func TestReadBasisRetainsEmptyCandidateAndInterpretationOnlyDependency(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	base, err := rd.Require(guard.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	// The second source contributes no selected paths; the third is only an
	// interpretation dependency. Neither may disappear from the fixed vector.
	repos := map[kernel.RepositoryID]objectmodel.Repository{
		base.ID(): &basisRepository{Repository: base, binding: "source-binding"},
	}
	for _, id := range []kernel.RepositoryID{"kr://basis/empty", "kr://basis/dependency"} {
		repos[id] = &basisRepository{Repository: basisRenamedRepository{Repository: base, id: id}, binding: kernel.Digest(id)}
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) { return repos[id], nil }
	pin := reader.DatasetPin{SetID: "orders", Revision: 1,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{base.ID(): at, "kr://basis/empty": at},
		Dependencies: map[kernel.RepositoryID]kernel.CommitID{"kr://basis/dependency": at},
		Items:        []reader.DatasetItem{{Repository: base.ID(), Commit: at, Kind: "file", File: "public/id.yaml"}}}
	basis, err := reader.Open(lookup, pin).ReadBasis(objectmodel.Reference{Object: "table/orders"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(basis.Candidates) != 2 || len(basis.Dependencies) != 1 {
		t.Fatalf("incomplete candidate vector: %#v", basis)
	}
	reopened, err := reader.OpenReadBasis(lookup, basis)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reopened.Pin().Repositories, pin.Repositories) || !reflect.DeepEqual(reopened.Pin().Dependencies, pin.Dependencies) {
		t.Fatalf("lost negative or dependency coordinates: %#v", reopened.Pin())
	}
	if _, err := reopened.Member("kr://basis/dependency"); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("interpretation dependency became a Dataset member: %v", err)
	}
}

func TestReadBasisWithUnresolvedInterpretationCannotFallBack(t *testing.T) {
	basis := objectmodel.ReadBasis{Version: 1, Reference: "urn:kc:object:table/orders",
		Candidates:             []objectmodel.ReadSource{{Repository: "kr://trade", Commit: "fixed", Binding: "authority"}},
		Paths:                  []objectmodel.ReadPath{{Repository: "kr://trade", Kind: "prefix"}},
		UnresolvedDependencies: []objectmodel.ReadDependencyFailure{{Repository: "kr://descriptors", Code: kernel.ErrTemporaryUnavailable}}}
	if err := basis.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.OpenReadBasis(func(kernel.RepositoryID) (objectmodel.Repository, error) {
		t.Fatal("an unresolved dependency must not be rediscovered or replaced by latest")
		return nil, nil
	}, basis); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("incomplete interpretation basis accepted: %v", err)
	}
}

// This fixture is metadata-only: it provides a second binding without deriving
// expected output from the production conversion.
type basisRenamedRepository struct {
	objectmodel.Repository
	id kernel.RepositoryID
}

func (r basisRenamedRepository) ID() kernel.RepositoryID { return r.id }
