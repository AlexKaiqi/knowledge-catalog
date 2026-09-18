package commandlog_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kc/kernel"
	"kc/snapshot/commandlog"
)

type memoryStore struct {
	entries []commandlog.Entry
	saves   int
	failAt  int
	failGet bool
}

func (s *memoryStore) Ready() error { return nil }

func (s *memoryStore) List() ([]commandlog.Entry, error) {
	return append([]commandlog.Entry(nil), s.entries...), nil
}

func (s *memoryStore) Get(commandID string) (commandlog.Entry, bool, error) {
	if s.failGet {
		return commandlog.Entry{}, false, errors.New("ledger read unavailable")
	}
	for _, entry := range s.entries {
		if entry.CommandID == commandID {
			return entry, true, nil
		}
	}
	return commandlog.Entry{}, false, nil
}

func TestCommandLogReadFailureCannotReapplyCommand(t *testing.T) {
	store := &memoryStore{failGet: true}
	ledger, err := commandlog.New(store)
	if err != nil {
		t.Fatal(err)
	}
	applied := 0
	_, _, err = ledger.Execute("cmd", "digest", commandlog.Request{Kind: "TEST"}, func() (any, error) {
		applied++
		return nil, nil
	})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || applied != 0 {
		t.Fatalf("ledger read failure = %v, applies=%d", err, applied)
	}
}

func (s *memoryStore) Put(entry commandlog.Entry) error {
	s.saves++
	if s.saves == s.failAt {
		return errors.New("disk unavailable")
	}
	for i := range s.entries {
		if s.entries[i].CommandID == entry.CommandID {
			s.entries[i] = entry
			return nil
		}
	}
	s.entries = append(s.entries, entry)
	return nil
}

func (s *memoryStore) Delete(commandID string) error {
	for i := range s.entries {
		if s.entries[i].CommandID == commandID {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			break
		}
	}
	return nil
}

func TestConcurrentIdenticalCommandAppliesOnce(t *testing.T) {
	ledger, err := commandlog.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	var applied atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := ledger.Execute("same", "digest", commandlog.Request{Kind: "TEST"}, func() (any, error) {
				applied.Add(1)
				return map[string]any{"ok": true}, nil
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := applied.Load(); got != 1 {
		t.Fatalf("apply count = %d, want 1", got)
	}
}

func TestCommandIDRejectsDifferentDigest(t *testing.T) {
	ledger, err := commandlog.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ledger.Execute("same", "a", commandlog.Request{Kind: "TEST"}, func() (any, error) {
		return map[string]any{"ok": true}, nil
	}); err != nil {
		t.Fatal(err)
	}
	_, _, err = ledger.Execute("same", "b", commandlog.Request{Kind: "TEST"}, func() (any, error) {
		t.Fatal("conflicting command must not apply")
		return nil, nil
	})
	if kernel.CodeOf(err) != kernel.ErrIdempotencyConflict {
		t.Fatalf("code = %s, err = %v", kernel.CodeOf(err), err)
	}
}

func TestDifferentCommandsMayApplyConcurrently(t *testing.T) {
	ledger, err := commandlog.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	errs := make(chan error, 2)
	for _, id := range []string{"a", "b"} {
		go func() {
			_, _, err := ledger.Execute(id, id, commandlog.Request{Kind: "TEST"}, func() (any, error) {
				started <- struct{}{}
				<-release
				return map[string]any{"id": id}, nil
			})
			errs <- err
		}()
	}
	<-started
	<-started
	close(release)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func TestReceiptSaveFailureDoesNotAllowDuplicateAfterRestart(t *testing.T) {
	store := &memoryStore{failAt: 2}
	ledger, err := commandlog.New(store)
	if err != nil {
		t.Fatal(err)
	}
	applied := 0
	_, _, err = ledger.Execute("cmd", "digest", commandlog.Request{Kind: "TEST"}, func() (any, error) {
		applied++
		return map[string]any{"commit": "abc"}, nil
	})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || applied != 1 {
		t.Fatalf("first result code=%s applied=%d err=%v", kernel.CodeOf(err), applied, err)
	}

	restarted, err := commandlog.New(store)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = restarted.Execute("cmd", "digest", commandlog.Request{Kind: "TEST"}, func() (any, error) {
		applied++
		return nil, nil
	})
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || applied != 1 {
		t.Fatalf("retry code=%s applied=%d err=%v", kernel.CodeOf(err), applied, err)
	}
}

func TestCommandLogRecoversCommitBeforeReceipt(t *testing.T) {
	store := &memoryStore{failAt: 2}
	ledger, err := commandlog.New(store)
	if err != nil {
		t.Fatal(err)
	}
	applied := 0
	_, _, err = ledger.Execute("cmd", "digest", commandlog.Request{Kind: "COMMIT"}, func() (any, error) {
		applied++
		return map[string]any{"commit": "accepted"}, nil
	})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || applied != 1 {
		t.Fatalf("crash point = %v, applies=%d", err, applied)
	}
	store.failAt = 0
	restarted, err := commandlog.New(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.ResolvePending("cmd", "digest", map[string]any{"commit": "accepted"}); err != nil {
		t.Fatal(err)
	}
	entry, replayed, err := restarted.Execute("cmd", "digest", commandlog.Request{Kind: "COMMIT"}, func() (any, error) {
		applied++
		return nil, nil
	})
	if err != nil || !replayed || applied != 1 {
		t.Fatalf("resolved replay = %#v replayed=%v applies=%d err=%v", entry, replayed, applied, err)
	}
}

func TestCommandLogRecoversReservationBeforeCommit(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	store := &memoryStore{entries: []commandlog.Entry{{
		CommandID: "reserved", Digest: "digest", Status: commandlog.StatusPending,
		CreatedAt: now, UpdatedAt: now, Request: commandlog.Request{Kind: "COMMIT"},
	}}}
	restarted, err := commandlog.New(store)
	if err != nil {
		t.Fatal(err)
	}
	applied := 0
	_, _, err = restarted.Execute("reserved", "digest", commandlog.Request{Kind: "COMMIT"}, func() (any, error) {
		applied++
		return nil, nil
	})
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || applied != 0 {
		t.Fatalf("recovered pre-commit reservation = %v, applies=%d", err, applied)
	}
	if _, err := restarted.AbandonPending("reserved", "digest"); err != nil {
		t.Fatal(err)
	}
}

func TestCommandLogRetentionIsBoundedAndKeepsPending(t *testing.T) {
	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	store := &memoryStore{entries: []commandlog.Entry{
		{CommandID: "applied", Digest: "a", Status: commandlog.StatusApplied, CreatedAt: old, UpdatedAt: old, Receipt: json.RawMessage(`{"ok":true}`)},
		{CommandID: "abandoned", Digest: "b", Status: commandlog.StatusAbandoned, CreatedAt: old, UpdatedAt: old},
		{CommandID: "pending", Digest: "c", Status: commandlog.StatusPending, CreatedAt: old, UpdatedAt: old},
	}}
	ledger, err := commandlog.New(store)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := ledger.Prune(time.Now().Add(-24 * time.Hour))
	if err != nil || removed != 2 {
		t.Fatalf("prune removed=%d err=%v", removed, err)
	}
	if _, ok, _ := store.Get("pending"); !ok {
		t.Fatal("retention inferred and deleted unresolved PENDING command")
	}
	if _, ok, _ := store.Get("applied"); ok {
		t.Fatal("expired applied command was retained")
	}
}

func TestCommandLogCanExplicitlyAbandonPending(t *testing.T) {
	old := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	store := &memoryStore{entries: []commandlog.Entry{{
		CommandID: "pending", Digest: "digest", Status: commandlog.StatusPending,
		CreatedAt: old, UpdatedAt: old,
	}}}
	ledger, err := commandlog.New(store)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := ledger.AbandonPending("pending", "digest")
	if err != nil || entry.Status != commandlog.StatusAbandoned {
		t.Fatalf("abandon = %#v, %v", entry, err)
	}
	_, _, err = ledger.Execute("pending", "digest", commandlog.Request{Kind: "TEST"}, func() (any, error) {
		t.Fatal("abandoned command must not apply")
		return nil, nil
	})
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("abandoned replay = %v", err)
	}
}

func TestBoltCommandLogPrunesWithoutDeletingPending(t *testing.T) {
	store := commandlog.NewBoltStore(filepath.Join(t.TempDir(), "commands.db"))
	ledger, err := commandlog.New(store)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	for _, entry := range []commandlog.Entry{
		{CommandID: "done", Digest: "a", Status: commandlog.StatusApplied, CreatedAt: old, UpdatedAt: old, Receipt: json.RawMessage(`{"ok":true}`)},
		{CommandID: "pending", Digest: "b", Status: commandlog.StatusPending, CreatedAt: old, UpdatedAt: old},
	} {
		if err := store.Put(entry); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := ledger.Prune(time.Now().Add(-24 * time.Hour))
	if err != nil || removed != 1 {
		t.Fatalf("Bolt prune removed=%d err=%v", removed, err)
	}
	if _, ok, err := store.Get("pending"); err != nil || !ok {
		t.Fatalf("pending after Bolt prune: ok=%v err=%v", ok, err)
	}
}

func TestApplyFailureReleasesDurableCommandClaim(t *testing.T) {
	store := &memoryStore{}
	ledger, err := commandlog.New(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ledger.Execute("cmd", "digest", commandlog.Request{Kind: "TEST"}, func() (any, error) {
		return nil, errors.New("validation failed")
	}); err == nil {
		t.Fatal("apply failure must be returned")
	}
	restarted, err := commandlog.New(store)
	if err != nil {
		t.Fatal(err)
	}
	applied := 0
	if _, _, err := restarted.Execute("cmd", "digest", commandlog.Request{Kind: "TEST"}, func() (any, error) {
		applied++
		return map[string]any{"ok": true}, nil
	}); err != nil || applied != 1 {
		t.Fatalf("released command did not retry: applied=%d err=%v", applied, err)
	}
}

func TestBoltStoreDoesNotImportLegacyJSON(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "writer.json")
	if err := os.WriteFile(legacy, []byte(`[{"commandId":"legacy","digest":"digest"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := commandlog.New(commandlog.NewBoltStore(filepath.Join(dir, "writer.db")))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ledger.Lookup("legacy"); ok {
		t.Fatal("new keyed ledger imported unsupported legacy JSON")
	}
}
