package observability_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kc/internal/jsonfile"
	"kc/kernel"
	"kc/knowledge"
	"kc/observability"
)

func datedFileStore(t *testing.T) *observability.FileStore {
	t.Helper()
	store := observability.NewFileStore(t.TempDir())
	store.Now = func() time.Time {
		return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	}
	return store
}

func TestFileStoreFailsClosedWhenPartitionPathIsAFile(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "access"), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := observability.NewFileStore(home)
	store.Now = func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) }
	_, err := store.RecordAccessReceipt(observability.AccessEvent{
		OccurredAt: "2026-09-16T00:00:00Z",
		Identity:   observability.IdentityContext{Principal: "agent:test"},
		Action:     "read", Decision: "ALLOW", Result: "RESOLVED",
	})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("blocked partition directory must fail closed: %v", err)
	}
}

func TestFileStoreDatePartitionHotDeleteAndQuota(t *testing.T) {
	store := datedFileStore(t)
	store.HotRetention = 30 * 24 * time.Hour
	identity := observability.IdentityContext{Principal: "agent:finance"}
	write := func(at string) string {
		t.Helper()
		id, err := store.RecordAccessReceipt(observability.AccessEvent{
			OccurredAt: at, Identity: identity, Action: "read", Decision: "ALLOW", Result: "RESOLVED",
			Knowledge: []observability.KnowledgeAccess{{KnowledgeRef: knowledge.PinnedKnowledgeRef{
				KnowledgeRef: knowledge.KnowledgeRef{Repository: "kr://acme/semantics", Object: "Metric:gmv"},
				Commit:       "c1",
			}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	oldID := write("2026-08-01T00:00:00Z")
	if _, err := os.Stat(filepath.Join(store.Home, "access", "2026-08-01.jsonl")); err != nil {
		t.Fatalf("expected UTC day partition: %v", err)
	}
	hotID := write("2026-09-10T00:00:00Z")
	if _, err := os.Stat(filepath.Join(store.Home, "access", "2026-08-01.jsonl")); !os.IsNotExist(err) {
		t.Fatal("closed day older than the hot window must be deleted")
	}
	if _, err := os.Stat(filepath.Join(store.Home, "access", "2026-09-10.jsonl")); err != nil {
		t.Fatalf("hot partition removed: %v", err)
	}
	if _, ok, err := store.GetAccess(context.Background(), oldID); err != nil || ok {
		t.Fatalf("pruned evidence must not be gettable: ok=%v err=%v", ok, err)
	}
	got, ok, err := store.GetAccess(context.Background(), hotID)
	if err != nil || !ok || got.EvidenceID != hotID {
		t.Fatalf("hot evidence missing: %#v ok=%v err=%v", got, ok, err)
	}

	page, err := store.Access(context.Background(), observability.AccessQuery{})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].EvidenceID != hotID {
		t.Fatalf("default query is the hot window: %#v err=%v", page, err)
	}

	store.MaxBytes = 1
	if _, err := store.RecordAccessReceipt(observability.AccessEvent{
		OccurredAt: "2026-09-16T00:00:00Z", Identity: identity, Action: "read", Decision: "ALLOW", Result: "RESOLVED",
	}); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("byte cap must fail closed: %v", err)
	}
}

func TestFileStoreFloodStageFailsClosed(t *testing.T) {
	store := datedFileStore(t)
	store.FloodUsedFraction = func() float64 { return 0.96 }
	_, err := store.RecordAccessReceipt(observability.AccessEvent{
		OccurredAt: "2026-09-16T00:00:00Z",
		Identity:   observability.IdentityContext{Principal: "agent:test"},
		Action:     "read", Decision: "ALLOW", Result: "RESOLVED",
	})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("disk flood-stage must fail closed: %v", err)
	}
}

func TestFileStoreMigratesLegacyJSONLIntoDayPartitions(t *testing.T) {
	home := t.TempDir()
	legacy := observability.AccessEvent{
		EvidenceID: "ev_legacy000000000000000000000000",
		OccurredAt: "2026-09-10T00:00:00Z",
		Identity:   observability.IdentityContext{Principal: "agent:legacy"},
		Action:     "read", Decision: "ALLOW", Result: "RESOLVED",
	}
	if err := jsonfile.AppendJSONL(filepath.Join(home, "access.jsonl"), legacy); err != nil {
		t.Fatal(err)
	}
	store := observability.NewFileStore(home)
	store.Now = func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) }
	got, ok, err := store.GetAccess(context.Background(), legacy.EvidenceID)
	if err != nil || !ok || got.Identity.Principal != "agent:legacy" {
		t.Fatalf("legacy jsonl must remain readable: %#v ok=%v err=%v", got, ok, err)
	}
	if _, err := store.RecordAccessReceipt(observability.AccessEvent{
		OccurredAt: "2026-09-16T00:00:00Z",
		Identity:   observability.IdentityContext{Principal: "agent:new"},
		Action:     "read", Decision: "ALLOW", Result: "RESOLVED",
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(home, "access.jsonl"))
	if err != nil || info.Size() != 0 {
		t.Fatalf("legacy file must be truncated to a volume marker: %#v %v", info, err)
	}
	raw, err := os.ReadFile(filepath.Join(home, "access", "2026-09-10.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var stored observability.AccessEvent
	if err := json.Unmarshal(raw, &stored); err != nil || stored.EvidenceID != legacy.EvidenceID {
		t.Fatalf("legacy event was not copied into the day partition: %s %v", raw, err)
	}
}

func TestParseRetentionAcceptsILMDayAndGoDuration(t *testing.T) {
	d, err := observability.ParseRetention("30d")
	if err != nil || d != 30*24*time.Hour {
		t.Fatalf("30d: %v %v", d, err)
	}
	d, err = observability.ParseRetention("720h")
	if err != nil || d != 30*24*time.Hour {
		t.Fatalf("720h: %v %v", d, err)
	}
	if err := observability.ValidateFileStorePolicy(observability.FileStorePolicy{HotRetention: "200d"}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("cap: %v", err)
	}
}
