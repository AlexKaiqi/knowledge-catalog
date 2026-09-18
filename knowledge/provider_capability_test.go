package knowledge_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
)

type nativeTreePoison struct {
	*testkit.KnowledgeRepository
}

func (*nativeTreePoison) NativeKnowledgeRepository() {}

func TestBaseAuthorityWithoutKnowledgeCapabilitiesFailsClosed(t *testing.T) {
	store := snapshot.NewRegistry()
	poison := testkit.NewCapabilityPoisonStore("kr://poison/base-only")
	if err := store.Add(poison); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.NewReader(store).Require(poison.ID(), kernel.ErrCapabilityUnsatisfied); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("base-only READ = %v", err)
	}
	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("poison-write", knowledge.ChangeSet{
		TargetRepository: poison.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: "poison-root", ExpectedTargetCommit: "poison-root",
		Operations: []knowledge.Operation{{
			Op:      knowledge.OpRemove,
			Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
		}},
	})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("base-only WRITE = %v", err)
	}
}

func TestNativeMarkerWithoutTypedChangeStoreCannotFallThroughToTreeEncoding(t *testing.T) {
	native := &nativeTreePoison{KnowledgeRepository: testkit.MakeRepository(t, "kr://poison/native-tree")}
	base, err := native.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	stores := snapshot.NewRegistry()
	if err := stores.Add(native); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(stores, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("native-tree-poison", knowledge.ChangeSet{
		TargetRepository: native.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpRemove, Address: knowledge.Address{
				Kind: knowledge.KindEntity, ObjectID: "policy/A",
			},
		}},
	})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("native marker without ChangeStore silently used tree encoding: %v", err)
	}
}
