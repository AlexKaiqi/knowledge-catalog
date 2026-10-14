package reader_test

import (
	"fmt"
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

type referenceReadGuard struct {
	snapshot.Store
	snapshot.TreeStore
	reads           map[string]int
	blocked         map[string]bool
	locatorOverride []byte
	commits         []kernel.CommitID
	beforeRead      func(string, kernel.CommitID)
}

func (g *referenceReadGuard) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	g.reads[path]++
	g.commits = append(g.commits, at)
	if g.beforeRead != nil {
		g.beforeRead(path, at)
	}
	if g.blocked[path] {
		return nil, fmt.Errorf("unselected object body read: %s", path)
	}
	if g.locatorOverride != nil && path == repofile.ObjectLocatorPath("table/orders") {
		return g.locatorOverride, nil
	}
	return g.TreeStore.ReadFile(path, at)
}

func (*referenceReadGuard) ListFiles(kernel.CommitID) ([]string, error) {
	return nil, fmt.Errorf("reference read must not scan a repository")
}

func newReferenceReadFixture(t *testing.T) (*reader.Reader, *referenceReadGuard, *writer.Writer, kernel.CommitID) {
	t.Helper()
	raw := testkit.MakeTreeStore(t, "kr://reader/references")
	writeRegistry := snapshot.NewRegistry()
	if err := writeRegistry.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(writeRegistry, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	receipt, err := w.Commit("seed-reference", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "description"}, PathHint: "private/description.yaml", Value: "订单表"},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "order_id"}, PathHint: "public/id.yaml", Value: map[string]any{"type": "string", "constraints": map[string]any{"nullable": false}, "optional": nil}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "status"}, PathHint: "private/status.yaml", Value: map[string]any{"type": "integer"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	guard := &referenceReadGuard{Store: raw, TreeStore: raw.(snapshot.TreeStore), reads: map[string]int{}, blocked: map[string]bool{}}
	readRegistry := snapshot.NewRegistry()
	if err := readRegistry.Add(guard); err != nil {
		t.Fatal(err)
	}
	return reader.NewReader(readRegistry), guard, w, receipt.Result.NewCommit
}

func TestReferenceReadPlansOnlySelectedMaintenanceUnits(t *testing.T) {
	for _, tc := range []struct {
		locator string
		path    string
		want    any
	}{
		{"/description", "private/description.yaml", "订单表"},
		{"/columns/order_id", "public/id.yaml", map[string]any{"type": "string", "constraints": map[string]any{"nullable": false}, "optional": nil}},
		{"/columns/order_id/constraints/nullable", "public/id.yaml", false},
		{"/columns/order_id/optional", "public/id.yaml", nil},
	} {
		t.Run(tc.locator, func(t *testing.T) {
			rd, guard, _, at := newReferenceReadFixture(t)
			for _, path := range []string{"private/description.yaml", "public/id.yaml", "private/status.yaml"} {
				guard.blocked[path] = path != tc.path
			}
			ref := objectmodel.Reference{Repository: guard.ID(), Object: "table/orders", Locator: tc.locator}
			got, err := rd.ReadReference(ref, at)
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := objectmodel.FormatReference(ref)
			if got.Reference != encoded || got.Locator != tc.locator || got.Repository != guard.ID() || got.Commit != at || !reflect.DeepEqual(got.Value, tc.want) {
				t.Fatalf("selected content or fixed reference changed: %#v", got)
			}
			if len(got.Declarations) != 1 || got.Digest != got.Declarations[0].Digest || got.Digest == "" {
				t.Fatalf("selection must retain its complete maintenance digest: %#v", got)
			}
			if tc.path == "public/id.yaml" && (got.Address.AspectName != "columns" || got.Address.MemberKey != "order_id" || got.Digest != kernel.CanonicalDigest(map[string]any{"type": "string", "constraints": map[string]any{"nullable": false}, "optional": nil})) {
				t.Fatalf("deep value used a leaf digest or lost its Member: %#v", got)
			}
			for path, blocked := range guard.blocked {
				if blocked && guard.reads[path] != 0 || !blocked && guard.reads[path] != 1 {
					t.Fatalf("selection read an incorrect unit set: %#v", guard.reads)
				}
			}
			for _, commit := range guard.commits {
				if commit != at {
					t.Fatalf("selection mixed fixed basis %s with %s", at, commit)
				}
			}
		})
	}
}

func TestReferenceCollectionHasNoSingleMaintenanceDigest(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	guard.blocked["private/description.yaml"] = true
	got, err := rd.ReadReference(objectmodel.Reference{Repository: guard.ID(), Object: "table/orders", Locator: "/columns"}, at)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"order_id": map[string]any{"type": "string", "constraints": map[string]any{"nullable": false}, "optional": nil},
		"status":   map[string]any{"type": "integer"},
	}
	if !reflect.DeepEqual(got.Value, want) || got.Digest != "" || len(got.Declarations) != 2 || guard.reads["private/description.yaml"] != 0 {
		t.Fatalf("member collection became a writable unit or loaded an unrelated Aspect: %#v reads=%#v", got, guard.reads)
	}
}

func TestDatasetReferenceReadKeepsSourceAndPublishedUnitScope(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	guard.blocked["private/description.yaml"], guard.blocked["private/status.yaml"] = true, true
	scope := reader.Open(rd.Lookup(func(kernel.RepositoryID) (snapshot.Store, error) { return guard, nil }), reader.DatasetPin{
		Repositories: map[kernel.RepositoryID]kernel.CommitID{guard.ID(): at},
		Items:        []reader.DatasetItem{{Repository: guard.ID(), Commit: at, Kind: "file", File: "public/id.yaml"}},
	})
	ref := objectmodel.Reference{Repository: guard.ID(), Object: "table/orders", Locator: "/columns/order_id/constraints/nullable"}
	values, err := scope.ReadReference(ref)
	if err != nil || len(values) != 1 || values[0].Repository != guard.ID() || values[0].Commit != at || values[0].Value != false || values[0].Address.MemberKey != "order_id" {
		t.Fatalf("published Member lost source coordinates or selected content: %#v %v", values, err)
	}
	for _, locator := range []string{"", "/description", "/columns", "/columns/status"} {
		ref.Locator = locator
		if _, err := scope.ReadReference(ref); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
			t.Fatalf("unpublished selection %q escaped scope: %v", locator, err)
		}
	}
	ref.Repository = "kr://reader/other-source"
	if _, err := scope.ReadReference(ref); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("Dataset guessed another source for a same-name object: %v", err)
	}
	if guard.reads["private/description.yaml"] != 0 || guard.reads["private/status.yaml"] != 0 {
		t.Fatalf("Dataset reference read private bodies: %#v", guard.reads)
	}
}

func TestReferenceSelectionRejectsLegacyLocatorBeforeReadingBodies(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	guard.locatorOverride, _ = repofile.EncodeObjectLocator("table/orders", []string{"private/description.yaml", "public/id.yaml", "private/status.yaml"})
	guard.blocked["private/description.yaml"], guard.blocked["private/status.yaml"], guard.blocked["public/id.yaml"] = true, true, true
	_, err := rd.ReadReference(objectmodel.Reference{Repository: guard.ID(), Object: "table/orders", Locator: "/columns/order_id"}, at)
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("legacy locator must report missing selection coordinates: %v", err)
	}
	for path := range guard.blocked {
		if guard.reads[path] != 0 {
			t.Fatalf("legacy selection inferred coordinates by loading bodies: %#v", guard.reads)
		}
	}
}

func TestReferenceReadKeepsRootAndCompleteMaintenanceBasis(t *testing.T) {
	rd, guard, w, before := newReferenceReadFixture(t)
	updated, err := w.Commit("change-member-type", objectmodel.ChangeSet{
		TargetRepository: guard.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: before, ExpectedTargetCommit: before,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "order_id"},
			Value:   map[string]any{"type": "integer", "constraints": map[string]any{"nullable": false}, "optional": nil}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := objectmodel.Reference{Repository: guard.ID(), Object: "table/orders"}
	root, err := rd.ReadReference(ref, before)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := map[string]any{"description": "订单表", "columns": map[string]any{
		"order_id": map[string]any{"type": "string", "constraints": map[string]any{"nullable": false}, "optional": nil},
		"status":   map[string]any{"type": "integer"},
	}}
	if root.Commit != before || !reflect.DeepEqual(root.Value, wantRoot) || root.Digest != kernel.CanonicalDigest(wantRoot) {
		t.Fatalf("root read changed old content or returned an incomplete digest: %#v", root)
	}
	ref.Locator = "/columns/order_id/constraints/nullable"
	oldLeaf, err := rd.ReadReference(ref, before)
	if err != nil {
		t.Fatal(err)
	}
	newLeaf, err := rd.ReadReference(ref, updated.Result.NewCommit)
	if err != nil {
		t.Fatal(err)
	}
	if oldLeaf.Value != false || newLeaf.Value != false || oldLeaf.Digest == newLeaf.Digest || oldLeaf.Address != newLeaf.Address || newLeaf.Commit != updated.Result.NewCommit {
		t.Fatalf("unchanged leaf hid another field's maintenance change: old=%#v new=%#v", oldLeaf, newLeaf)
	}
	trimmed, err := rd.Read(objectmodel.ObjectRef{Repository: guard.ID(), Object: "table/orders"}, before, &objectmodel.AspectSelector{Include: []string{"description"}})
	if err != nil || trimmed.Digest != kernel.CanonicalDigest(wantRoot) || !reflect.DeepEqual(trimmed.Value, map[string]any{"description": "订单表"}) {
		t.Fatalf("legacy selected READ lost the complete object precondition: %#v %v", trimmed, err)
	}
	unit, err := rd.ReadAddress(guard.ID(), oldLeaf.Address, before)
	if err != nil || unit.Digest != oldLeaf.Digest {
		t.Fatalf("legacy unit READ lost its complete maintenance precondition: %#v %v", unit, err)
	}
}

func TestReferenceReadDoesNotFollowHeadWhileLoadingSelectedBody(t *testing.T) {
	rd, guard, w, before := newReferenceReadFixture(t)
	var newer kernel.CommitID
	guard.beforeRead = func(path string, _ kernel.CommitID) {
		if path != "public/id.yaml" || newer != "" {
			return
		}
		updated, err := w.Commit("advance-during-reference", objectmodel.ChangeSet{
			TargetRepository: guard.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: before, ExpectedTargetCommit: before,
			Operations: []objectmodel.Operation{{Op: objectmodel.OpPut,
				Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "order_id"},
				Value:   map[string]any{"type": "integer", "constraints": map[string]any{"nullable": true}}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		newer = updated.Result.NewCommit
	}
	value, err := rd.ReadReference(objectmodel.Reference{Repository: guard.ID(), Object: "table/orders", Locator: "/columns/order_id/constraints/nullable"}, before)
	if err != nil || newer == "" || newer == before || value.Commit != before || value.Value != false {
		t.Fatalf("reference followed HEAD while reading: value=%#v newer=%s err=%v", value, newer, err)
	}
	for _, at := range guard.commits {
		if at != before {
			t.Fatalf("reference mixed mutable HEAD with fixed basis: %v", guard.commits)
		}
	}
}

func TestReferenceReadRejectsCoordinatesThatDisagreeWithUnit(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	locations, err := repofile.ReadObjectUnitLocations(guard.TreeStore, "table/orders", at)
	if err != nil {
		t.Fatal(err)
	}
	for i := range locations {
		if locations[i].Address.MemberKey == "order_id" {
			locations[i].Address.MemberKey = "wrong-member"
		}
	}
	guard.locatorOverride, err = repofile.EncodeObjectUnitLocations("table/orders", locations)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rd.ReadReference(objectmodel.Reference{Repository: guard.ID(), Object: "table/orders", Locator: "/columns/wrong-member"}, at)
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("reference accepted coordinates different from the stored unit: %v", err)
	}
}

func TestReferenceAuthorizationPlanUsesFixedCoordinatesWithoutBodies(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	for _, path := range []string{"private/description.yaml", "public/id.yaml", "private/status.yaml"} {
		guard.blocked[path] = true
	}
	ref := objectmodel.Reference{Repository: guard.ID(), Object: "table/orders", Locator: "/columns/order_id/constraints/nullable"}
	addresses, err := rd.PlanReference(ref, at)
	if err != nil || len(addresses) != 1 || addresses[0].Kind != objectmodel.KindMember || addresses[0].MemberKey != "order_id" || addresses[0].AspectName != "columns" {
		t.Fatalf("metadata plan lost exact maintained coordinates: %#v %v", addresses, err)
	}
	ref.Locator = ""
	addresses, err = rd.PlanReference(ref, at)
	if err != nil || len(addresses) != 3 {
		t.Fatalf("root metadata plan lost existing representation: %#v %v", addresses, err)
	}
	for path := range guard.blocked {
		if guard.reads[path] != 0 {
			t.Fatalf("authorization plan read object content: %#v", guard.reads)
		}
	}
	for _, commit := range guard.commits {
		if commit != at {
			t.Fatalf("metadata plan mixed fixed commits %s/%s", at, commit)
		}
	}
}
