package writer_test

import (
	"testing"

	"kc/internal/repofile"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
)

func TestExplicitLocatorRebuildRecoversLegacyLayout(t *testing.T) {
	raw := testkit.MakeTreeStore(t, "kr://writer/locator-rebuild")
	tree := raw.(snapshot.TreeStore)
	registry := snapshot.NewRegistry()
	if err := registry.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	seed, err := w.Commit("seed", knowledge.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: testkit.PutEntity("policy/A", map[string]any{"version": 1}, "policies/A.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyRaw, err := repofile.EncodeLocatorManifest(repofile.LocatorManifest{
		Objects: map[knowledge.ObjectID][]string{"policy/A": {"policies/A.json"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: seed.Result.CommitID, ExpectedTargetCommit: seed.Result.CommitID,
		Changes: []snapshot.TreeChange{
			{Path: repofile.ObjectLocatorPath("policy/A"), Remove: true},
			{Path: repofile.LocatorCompletePath, Remove: true},
			{Path: repofile.LocatorManifestPath, Content: legacyRaw},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("blocked", knowledge.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: legacy, ExpectedTargetCommit: legacy,
		Operations: testkit.PutEntity("policy/A", map[string]any{"version": 2}, ""),
	})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("legacy write did not fail closed: %v", err)
	}
	rebuilt, err := w.RebuildTreeLocators("rebuild", raw.ID(), snapshot.DefaultRef, legacy)
	if err != nil || rebuilt.ObjectCount != 1 || rebuilt.NewCommit == "" {
		t.Fatalf("rebuild = %#v, %v", rebuilt, err)
	}
	updated, err := w.Commit("update", knowledge.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: rebuilt.NewCommit, ExpectedTargetCommit: rebuilt.NewCommit,
		Operations: testkit.PutEntity("policy/A", map[string]any{"version": 2}, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := reader.NewReader(registry).Require(raw.ID(), kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read("policy/A", updated.Result.CommitID)
	if err != nil || value.Value.(map[string]any)["version"] == nil {
		t.Fatalf("read after locator rebuild = %#v, %v", value, err)
	}
}
