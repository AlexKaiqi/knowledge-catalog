package object_test

import (
	"fmt"
	"testing"
	"time"

	objectmodel "kc/object"
)

func applyUnitsBatch(objects int) ([]objectmodel.Unit, []objectmodel.Operation) {
	existing := make([]objectmodel.Unit, 0, objects)
	operations := make([]objectmodel.Operation, 0, objects)
	for index := 0; index < objects; index++ {
		address := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(fmt.Sprintf("note/%06d", index))}
		existing = append(existing, objectmodel.Unit{ObjectID: address.ObjectID, Address: address, Value: map[string]any{"v": index}})
		operations = append(operations, objectmodel.Operation{Op: objectmodel.OpPut, Address: address, Value: map[string]any{"v": index + 1}})
	}
	return existing, operations
}

func timeApplyUnits(t *testing.T, objects int) time.Duration {
	t.Helper()
	existing, operations := applyUnitsBatch(objects)
	best := time.Duration(0)
	for range 3 {
		started := time.Now()
		final, _, err := objectmodel.ApplyUnits(existing, operations, nil)
		elapsed := time.Since(started)
		if err != nil || len(final) != objects {
			t.Fatalf("apply %d units: final=%d err=%v", objects, len(final), err)
		}
		if best == 0 || elapsed < best {
			best = elapsed
		}
	}
	return best
}

// ApplyUnits used to scan every unit of the batch for each operation to find
// siblings, so a full-repository update was quadratic. Eight times the batch
// must cost far less than sixty-four times the time.
func TestApplyUnitsScalesLinearly(t *testing.T) {
	small, large := timeApplyUnits(t, 1000), timeApplyUnits(t, 8000)
	if ratio := float64(large) / float64(small); ratio > 24 {
		t.Fatalf("ApplyUnits grew %.1fx for 8x objects (%s -> %s)", ratio, small, large)
	}
}

// Removing an entity removes every sibling unit of that object and only those,
// and a digest precondition still sees the whole sibling set.
func TestApplyUnitsRemoveEntityDropsAllSiblingsOnly(t *testing.T) {
	keep := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/keep"}
	root := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/drop"}
	first := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "note/drop", AspectName: "first"}
	second := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "note/drop", AspectName: "second"}
	existing := []objectmodel.Unit{
		{ObjectID: keep.ObjectID, Address: keep, Value: map[string]any{"k": 1}},
		{ObjectID: first.ObjectID, Address: first, Value: map[string]any{"f": 1}},
		{ObjectID: second.ObjectID, Address: second, Value: map[string]any{"s": 1}},
	}
	digest, err := objectmodel.UnitSetMaintenanceDigest(existing[1:])
	if err != nil {
		t.Fatal(err)
	}
	stale := []objectmodel.Operation{{Op: objectmodel.OpRemove, Address: root,
		Precondition: &objectmodel.Precondition{Type: objectmodel.IfObjectEquals, Digest: "stale"}}}
	if _, _, err := objectmodel.ApplyUnits(existing, stale, nil); err == nil {
		t.Fatal("stale whole-object digest precondition was accepted")
	}
	remove := []objectmodel.Operation{{Op: objectmodel.OpRemove, Address: root,
		Precondition: &objectmodel.Precondition{Type: objectmodel.IfObjectEquals, Digest: digest}}}
	final, deleted, err := objectmodel.ApplyUnits(existing, remove, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(final) != 1 || final[0].ObjectID != keep.ObjectID || len(deleted) != 2 {
		t.Fatalf("remove entity: final=%v deleted=%v", final, deleted)
	}
	again := []objectmodel.Operation{{Op: objectmodel.OpRemove, Address: root}, {Op: objectmodel.OpRemove, Address: root}}
	if _, _, err := objectmodel.ApplyUnits(existing, again, nil); err == nil {
		t.Fatal("second remove of an already removed object was accepted")
	}
}
