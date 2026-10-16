package writer_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func newBindingMaintenance(t *testing.T) (testkit.Setup, kernel.CommitID, objectmodel.Operation, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"rowCount":999}`))
	}))
	t.Cleanup(origin.Close)
	s := testkit.NewSetup(t, "kr://writer/binding-maintenance")
	op := bindingfixture.PutMember("table/orders", "runtime", "row-count", "", "resource/warehouse")
	at, err := commitOps(t, s, s.RootCommitID,
		bindingfixture.PutDescriptor("resource/warehouse", bindingfixture.Descriptor(origin.URL)),
		bindingfixture.PutDescriptor("resource/pipeline", bindingfixture.Descriptor(origin.URL)), op,
		objectmodel.Operation{Op: objectmodel.OpPut, Address: maintenanceDescription, Value: map[string]any{"text": "orders"}})
	if err != nil {
		t.Fatal(err)
	}
	return s, at, op, &calls
}

func TestBindingMaintenanceRejectsStaleDeclarationWithCurrentRepositoryBasis(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(map[bool]string{false: "put", true: "remove"}[remove], func(t *testing.T) {
			s, at, op, calls := newBindingMaintenance(t)
			basis, err := s.Reader.ResolveAddress(s.RepositoryID, op.Address, at)
			if err != nil {
				t.Fatal(err)
			}
			op.ValueSource = bindingfixture.PutMember("table/orders", "runtime", "row-count", "", "resource/pipeline").ValueSource
			op.Precondition = &objectmodel.Precondition{Type: objectmodel.IfDigestEquals, Digest: basis.Digest}
			changed, err := commitOps(t, s, at, op)
			if err != nil {
				t.Fatalf("fresh declaration basis rejected: %v", err)
			}
			if remove {
				op.Op = objectmodel.OpRemove
			}
			_, err = commitOps(t, s, changed, op)
			testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
			if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != changed {
				t.Fatalf("stale declaration changed HEAD: %s != %s", got, changed)
			}
			current, err := s.Reader.ResolveAddress(s.RepositoryID, op.Address, changed)
			if err != nil || current.Digest == basis.Digest {
				t.Fatalf("local declaration basis did not change: %#v %v", current, err)
			}
			op.Precondition.Digest = current.Digest
			if _, err := commitOps(t, s, changed, op); err != nil {
				t.Fatalf("current declaration basis rejected: %v", err)
			}
			if calls.Load() != 0 {
				t.Fatalf("maintenance called live origin %d times", calls.Load())
			}
		})
	}
}

func TestMixedObjectRemovalRejectsStaleBindingDeclaration(t *testing.T) {
	s, at, op, calls := newBindingMaintenance(t)
	ref := objectmodel.ObjectRef{Repository: s.RepositoryID, Object: op.Address.ObjectID}
	basis, err := s.Reader.Resolve(ref, at)
	if err != nil {
		t.Fatal(err)
	}
	op.ValueSource = bindingfixture.PutMember("table/orders", "runtime", "row-count", "", "resource/pipeline").ValueSource
	changed, err := commitOps(t, s, at, op)
	if err != nil {
		t.Fatal(err)
	}
	remove := objectmodel.Operation{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: ref.Object},
		Precondition: &objectmodel.Precondition{Type: objectmodel.IfDigestEquals, Digest: basis.Digest}}
	_, err = commitOps(t, s, changed, remove)
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != changed {
		t.Fatalf("stale whole-object removal changed HEAD: %s", got)
	}
	current, err := s.Reader.Resolve(ref, changed)
	if err != nil || current.Digest == basis.Digest {
		t.Fatalf("whole-object basis did not change: %#v %v", current, err)
	}
	remove.Precondition.Digest = current.Digest
	if _, err := commitOps(t, s, changed, remove); err != nil {
		t.Fatalf("fresh whole-object removal rejected: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("maintenance called live origin %d times", calls.Load())
	}
}

type maintenanceNativeProvider struct {
	snapshot.Store
	objectmodel.ReadStore
}

func (*maintenanceNativeProvider) NativeObjectRepository() {}

func TestMaintenanceResolutionKeepsContentHashesAndNativeProviderBasisSeparate(t *testing.T) {
	s, at, op, calls := newBindingMaintenance(t)
	want := objectmodel.DeclarationDigest(op.SchemaRef, op.ValueSource)
	first, err := s.Reader.ResolveAddress(s.RepositoryID, op.Address, at)
	if err != nil || first.Digest != want {
		t.Fatalf("binding maintenance basis = %#v %v; want %s", first, err, want)
	}
	changed, err := commitOps(t, s, at, bindingfixture.PutDescriptor("resource/warehouse", bindingfixture.Descriptor("https://changed.example")))
	if err != nil {
		t.Fatal(err)
	}
	store, _ := s.Store.Get(s.RepositoryID)
	native, err := reader.NewReader(nil).Wrap(&maintenanceNativeProvider{Store: store, ReadStore: s.Repo}, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	for name, repo := range map[string]objectmodel.Repository{"tree": s.Repo, "native": native} {
		t.Run(name, func(t *testing.T) {
			resolved, err := repo.ResolveAddress(op.Address, changed)
			if err != nil || resolved.Digest != want {
				t.Fatalf("descriptor version changed local maintenance basis: %#v %v", resolved, err)
			}
			value, err := repo.ReadAddress(op.Address, changed)
			if err != nil || len(value.Declarations) != 1 || value.Declarations[0].Digest != kernel.CanonicalDigest(nil) {
				t.Fatalf("declaration content hash changed: %#v %v", value, err)
			}
			root, err := repo.Resolve(op.Address.ObjectID, changed)
			if err != nil || root.Digest == "" || root.Digest == kernel.CanonicalDigest(map[string]any{"runtime": map[string]any{"row-count": nil}, "description": map[string]any{"text": "orders"}}) {
				t.Fatalf("mixed root lost its declaration basis: %#v %v", root, err)
			}
		})
	}
	paths := s.Repo.Repository.(objectmodel.AddressPathsReader)
	resolved, err := paths.ResolveAddressAtPaths(op.Address, at, []string{first.PathHint})
	if err != nil || resolved.Digest != want {
		t.Fatalf("path-limited resolve changed maintenance basis: %#v %v", resolved, err)
	}
	content, err := s.Reader.ReadAddress(s.RepositoryID, op.Address, changed)
	if err != nil || content.Digest != kernel.CanonicalDigest(nil) {
		t.Fatalf("content READ hash changed: %#v %v", content, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("maintenance called live origin %d times", calls.Load())
	}
}
