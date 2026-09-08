package home

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"kc/knowledge"
	"kc/snapshot"
)

func TestHomeWiresCacheAndRecoversWarmupWithoutSearchProvider(t *testing.T) {
	cfg := deploymentFixture(t)
	on := true
	cfg.Stores.Index = "none"
	cfg.Stores.HydrationCache = &HydrationCacheConfig{MaxEntries: 16, WarmLimit: 4, ColdStart: &on}
	seed := func(dir, _ string) error {
		return os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600)
	}
	if err := InitializeDeployment(cfg, seed); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		ws, err := OpenDeployment(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if ws.Hydrator == nil || ws.ReadCache == nil || ws.Projection == nil {
			t.Fatal("cache-only deployment lacks hydration and maintenance wiring")
		}
		if stats := ws.ReadCache.Stats(); stats.Entries != 0 || stats.WarmRuns != 0 {
			t.Fatalf("Open started maintenance: %+v", stats)
		}
		if err := ws.Projection.CatchUp(context.Background()); err != nil {
			t.Fatal(err)
		}
		targets, err := ws.Projection.ConsumerTargets()
		if err != nil || len(targets) == 0 {
			t.Fatalf("independent warm target absent: %+v %v", targets, err)
		}
		repo, err := ws.Reader.Require(knowledge.SystemRepositoryID, "CAPABILITY_UNSATISFIED")
		if err != nil {
			t.Fatal(err)
		}
		head, err := repo.Head(snapshot.DefaultRef)
		if err != nil {
			t.Fatal(err)
		}
		id := knowledge.SystemSchemaOperations()[0].Address.ObjectID
		ref := knowledge.KnowledgeRef{Repository: repo.ID(), Object: id}
		if _, err := ws.Reader.Read(ref, head, nil); err != nil {
			t.Fatal(err)
		}
		before := ws.ReadCache.Stats()
		if _, err := ws.ReadView(nil).Reader.Read(ref, head, nil); err != nil {
			t.Fatal(err)
		}
		after := ws.ReadCache.Stats()
		if after.Hits <= before.Hits || after.SourceReads != before.SourceReads {
			t.Fatalf("Home reader did not reuse body: %+v -> %+v", before, after)
		}
		if _, err := ws.Index.CheckSearchProjectionAt(repo, head); err == nil {
			t.Fatal("cache warmup fabricated search readiness")
		}
		if err := ws.Close(); err != nil {
			t.Fatal(err)
		}
		if !ws.ReadCache.Stats().Closed {
			t.Fatal("Home did not close its cache")
		}
	}
}

func TestHydrationSettingsRoundTripAndRejectNegativeLimits(t *testing.T) {
	on := true
	dir := t.TempDir()
	settings := DefaultStores()
	settings.HydrationCache = &HydrationCacheConfig{MaxBytes: 8192, MaxEntries: 7, WarmLimit: 3, WarmBatchSize: 2, ColdStart: &on}
	if err := WriteStores(dir, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadStores(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := loaded.HydrationCache
	if c == nil || c.MaxBytes != 8192 || c.MaxEntries != 7 || c.WarmLimit != 3 || c.WarmBatchSize != 2 || c.ColdStart == nil || !*c.ColdStart {
		t.Fatalf("cache settings lost: %+v", c)
	}
	c.MaxBytes = -1
	if err := loaded.ValidateProfile(); err == nil {
		t.Fatal("negative budget accepted")
	}
}
