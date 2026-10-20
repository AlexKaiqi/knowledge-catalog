package snapshot_test

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
	"kc/snapshot"
)

type memoryStore struct {
	entries    []snapshot.CommandEntry
	saves      int
	failAt     int
	failGet    bool
	failDelete bool
}

func (s *memoryStore) Ready() error { return nil }

func (s *memoryStore) List() ([]snapshot.CommandEntry, error) {
	return append([]snapshot.CommandEntry(nil), s.entries...), nil
}

func (s *memoryStore) Get(commandID string) (snapshot.CommandEntry, bool, error) {
	if s.failGet {
		return snapshot.CommandEntry{}, false, errors.New("ledger read unavailable")
	}
	for _, entry := range s.entries {
		if entry.CommandID == commandID {
			return entry, true, nil
		}
	}
	return snapshot.CommandEntry{}, false, nil
}

func TestCommandLogReadFailureCannotReapplyCommand(t *testing.T) {
	store := &memoryStore{failGet: true}
	ledger, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	applied := 0
	_, _, err = ledger.Execute("cmd", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
		applied++
		return nil, nil
	})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || applied != 0 {
		t.Fatalf("ledger read failure = %v, applies=%d", err, applied)
	}
}

func (s *memoryStore) Put(entry snapshot.CommandEntry) error {
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
	if s.failDelete {
		return errors.New("ledger delete unavailable")
	}
	for i := range s.entries {
		if s.entries[i].CommandID == commandID {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			break
		}
	}
	return nil
}

func TestConcurrentIdenticalCommandAppliesOnce(t *testing.T) {
	ledger, err := snapshot.NewCommandLedger(nil)
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
			_, _, err := ledger.Execute("same", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
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
	ledger, err := snapshot.NewCommandLedger(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ledger.Execute("same", "a", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
		return map[string]any{"ok": true}, nil
	}); err != nil {
		t.Fatal(err)
	}
	_, _, err = ledger.Execute("same", "b", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
		t.Fatal("conflicting command must not apply")
		return nil, nil
	})
	if kernel.CodeOf(err) != kernel.ErrIdempotencyConflict {
		t.Fatalf("code = %s, err = %v", kernel.CodeOf(err), err)
	}
}

func TestDifferentCommandsMayApplyConcurrently(t *testing.T) {
	ledger, err := snapshot.NewCommandLedger(nil)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	errs := make(chan error, 2)
	for _, id := range []string{"a", "b"} {
		go func() {
			_, _, err := ledger.Execute(id, id, snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
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
	ledger, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	applied := 0
	_, _, err = ledger.Execute("cmd", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
		applied++
		return map[string]any{"commit": "abc"}, nil
	})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || applied != 1 {
		t.Fatalf("first result code=%s applied=%d err=%v", kernel.CodeOf(err), applied, err)
	}
	if kernel.IsWriteNotApplied(err) || kernel.Normalize(err).WriteOutcome != "" {
		t.Fatal("receipt persistence failure after apply was mislabeled as unapplied")
	}

	restarted, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = restarted.Execute("cmd", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
		applied++
		return nil, nil
	})
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || applied != 1 {
		t.Fatalf("retry code=%s applied=%d err=%v", kernel.CodeOf(err), applied, err)
	}
}

func TestCommandLogRecoversCommitBeforeReceipt(t *testing.T) {
	store := &memoryStore{failAt: 2}
	ledger, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	applied := 0
	_, _, err = ledger.Execute("cmd", "digest", snapshot.CommandRequest{Kind: "COMMIT"}, func() (any, error) {
		applied++
		return map[string]any{"commit": "accepted"}, nil
	})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || applied != 1 {
		t.Fatalf("crash point = %v, applies=%d", err, applied)
	}
	store.failAt = 0
	restarted, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.ResolvePending("cmd", "digest", map[string]any{"commit": "accepted"}); err != nil {
		t.Fatal(err)
	}
	entry, replayed, err := restarted.Execute("cmd", "digest", snapshot.CommandRequest{Kind: "COMMIT"}, func() (any, error) {
		applied++
		return nil, nil
	})
	if err != nil || !replayed || applied != 1 {
		t.Fatalf("resolved replay = %#v replayed=%v applies=%d err=%v", entry, replayed, applied, err)
	}
}

func TestCommandLogRecoversReservationBeforeCommit(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	store := &memoryStore{entries: []snapshot.CommandEntry{{
		CommandID: "reserved", Digest: "digest", Status: snapshot.StatusPending,
		CreatedAt: now, UpdatedAt: now, Request: snapshot.CommandRequest{Kind: "COMMIT"},
	}}}
	restarted, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	applied := 0
	_, _, err = restarted.Execute("reserved", "digest", snapshot.CommandRequest{Kind: "COMMIT"}, func() (any, error) {
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
	store := &memoryStore{entries: []snapshot.CommandEntry{
		{CommandID: "applied", Digest: "a", Status: snapshot.StatusApplied, CreatedAt: old, UpdatedAt: old, Receipt: json.RawMessage(`{"ok":true}`)},
		{CommandID: "abandoned", Digest: "b", Status: snapshot.StatusAbandoned, CreatedAt: old, UpdatedAt: old},
		{CommandID: "pending", Digest: "c", Status: snapshot.StatusPending, CreatedAt: old, UpdatedAt: old},
	}}
	ledger, err := snapshot.NewCommandLedger(store)
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
	store := &memoryStore{entries: []snapshot.CommandEntry{{
		CommandID: "pending", Digest: "digest", Status: snapshot.StatusPending,
		CreatedAt: old, UpdatedAt: old,
	}}}
	ledger, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := ledger.AbandonPending("pending", "digest")
	if err != nil || entry.Status != snapshot.StatusAbandoned {
		t.Fatalf("abandon = %#v, %v", entry, err)
	}
	_, _, err = ledger.Execute("pending", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
		t.Fatal("abandoned command must not apply")
		return nil, nil
	})
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("abandoned replay = %v", err)
	}
}

func TestBoltCommandLogPrunesWithoutDeletingPending(t *testing.T) {
	store := snapshot.NewBoltCommandStore(filepath.Join(t.TempDir(), "commands.db"))
	ledger, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	for _, entry := range []snapshot.CommandEntry{
		{CommandID: "done", Digest: "a", Status: snapshot.StatusApplied, CreatedAt: old, UpdatedAt: old, Receipt: json.RawMessage(`{"ok":true}`)},
		{CommandID: "pending", Digest: "b", Status: snapshot.StatusPending, CreatedAt: old, UpdatedAt: old},
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
	ledger, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ledger.Execute("cmd", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
		return nil, snapshot.WriteNotApplied(errors.New("validation failed before publication"))
	}); err == nil {
		t.Fatal("apply failure must be returned")
	}
	restarted, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	applied := 0
	if _, _, err := restarted.Execute("cmd", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
		applied++
		return map[string]any{"ok": true}, nil
	}); err != nil || applied != 1 {
		t.Fatalf("released command did not retry: applied=%d err=%v", applied, err)
	}
}

func TestUnknownApplyFailureKeepsDurableCommandClaim(t *testing.T) {
	store := &memoryStore{}
	ledger, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	outcome := kernel.Fail(kernel.ErrTemporaryUnavailable, "lost publication response")
	_, _, err = ledger.Execute("cmd", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
		return nil, outcome
	})
	if !errors.Is(err, outcome) {
		t.Fatalf("unknown error was replaced: %v", err)
	}
	if kernel.IsWriteNotApplied(err) || kernel.Normalize(err).WriteOutcome != "" {
		t.Fatal("unknown publication outcome was relabeled as unapplied")
	}
	restarted, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := restarted.Lookup("cmd")
	if !ok || entry.Status != snapshot.StatusPending || entry.Digest != "digest" {
		t.Fatalf("unknown failure lost claim: %+v, found = %v", entry, ok)
	}
	for _, tc := range []struct {
		digest string
		code   kernel.ErrorCode
	}{{"digest", kernel.ErrPreconditionFailed}, {"different", kernel.ErrIdempotencyConflict}} {
		_, _, err = restarted.Execute("cmd", tc.digest, snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
			t.Fatal("unknown result must not execute again")
			return nil, nil
		})
		if kernel.CodeOf(err) != tc.code {
			t.Fatalf("retry digest %s: %v", tc.digest, err)
		}
	}
}

func TestUnappliedCommandDeleteFailureKeepsPendingClaim(t *testing.T) {
	store := &memoryStore{failDelete: true}
	ledger, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ledger.Execute("cmd", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
		return nil, snapshot.WriteNotApplied(kernel.Fail(kernel.ErrNonFastForward, "ref moved before publication"))
	})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("failed durable release = %v", err)
	}
	if !kernel.IsWriteNotApplied(err) || kernel.Normalize(err).WriteOutcome != kernel.WriteOutcomeNotApplied {
		t.Fatalf("claim cleanup failure erased the proven publication outcome: %v", err)
	}
	if entry, ok := ledger.Lookup("cmd"); !ok || entry.Status != snapshot.StatusPending {
		t.Fatalf("failed durable release discarded in-memory claim: %+v, found = %v", entry, ok)
	}
	_, _, retryErr := ledger.Execute("cmd", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
		t.Fatal("unapplied proof is not permission to replay a retained claim")
		return nil, nil
	})
	if kernel.CodeOf(retryErr) != kernel.ErrPreconditionFailed {
		t.Fatalf("retained command allowed same-request retry: %v", retryErr)
	}
	_, _, err = ledger.Execute("cmd", "different", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) {
		t.Fatal("failed claim release must not execute a new payload")
		return nil, nil
	})
	if kernel.CodeOf(err) != kernel.ErrIdempotencyConflict {
		t.Fatalf("failed release lost digest protection: %v", err)
	}
}

func TestWriteNotAppliedPreservesProtocolFailure(t *testing.T) {
	original := kernel.Fail(kernel.ErrNonFastForward, "ref moved")
	wrapped := snapshot.WriteNotApplied(original)
	if !errors.Is(wrapped, original) || kernel.CodeOf(wrapped) != kernel.ErrNonFastForward || wrapped.Error() != original.Error() {
		t.Fatalf("unapplied proof changed error: %v", wrapped)
	}
	if snapshot.WriteNotApplied(wrapped) != wrapped || snapshot.WriteNotApplied(nil) != nil {
		t.Fatal("unapplied proof did not preserve existing wrapper or nil")
	}
}

func TestBoltPendingNullReceiptRequiresExplicitResolution(t *testing.T) {
	for _, resolve := range []bool{false, true} {
		name := "abandon"
		if resolve {
			name = "resolve"
		}
		t.Run(name, func(t *testing.T) {
			store := snapshot.NewBoltCommandStore(filepath.Join(t.TempDir(), "commands.db"))
			if err := store.Ready(); err != nil {
				t.Fatal(err)
			}
			if err := store.Put(snapshot.CommandEntry{CommandID: "pending", Digest: "digest", Status: snapshot.StatusPending}); err != nil {
				t.Fatal(err)
			}
			ledger, err := snapshot.NewCommandLedger(store)
			if err != nil {
				t.Fatal(err)
			}
			apply := func() (any, error) {
				t.Fatal("durable pending command must never reapply")
				return nil, nil
			}
			_, _, err = ledger.Execute("pending", "digest", snapshot.CommandRequest{Kind: "TEST"}, apply)
			if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("null pending receipt replayed before explicit decision: %v", err)
			}
			if resolve {
				if _, err := ledger.ResolvePending("pending", "digest", map[string]any{"commit": "accepted"}); err != nil {
					t.Fatal(err)
				}
			} else if _, err := ledger.AbandonPending("pending", "digest"); err != nil {
				t.Fatal(err)
			}
			reopened, err := snapshot.NewCommandLedger(store)
			if err != nil {
				t.Fatal(err)
			}
			entry, replayed, err := reopened.Execute("pending", "digest", snapshot.CommandRequest{Kind: "TEST"}, apply)
			if resolve {
				if err != nil || !replayed || entry.Status != snapshot.StatusApplied || string(entry.Receipt) != `{"commit":"accepted"}` {
					t.Fatalf("resolved command did not replay accepted receipt: %+v, replayed = %v, error = %v", entry, replayed, err)
				}
			} else if replayed || kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("abandoned command replayed: %+v, replayed = %v, error = %v", entry, replayed, err)
			}
		})
	}
}

func TestBoltStoreDoesNotImportLegacyJSON(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "writer.json")
	if err := os.WriteFile(legacy, []byte(`[{"commandId":"legacy","digest":"digest"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := snapshot.NewCommandLedger(snapshot.NewBoltCommandStore(filepath.Join(dir, "writer.db")))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ledger.Lookup("legacy"); ok {
		t.Fatal("new keyed ledger imported unsupported legacy JSON")
	}
}

func TestUnappliedProofSurvivesProtocolErrorEnvelope(t *testing.T) {
	original := kernel.Fail(kernel.ErrTemporaryUnavailable, "pre-publication staging failed")
	ledger, err := snapshot.NewCommandLedger(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ledger.Execute("unapplied-wire", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) { return nil, snapshot.WriteNotApplied(original) })
	if !errors.Is(err, original) {
		t.Fatalf("original failure lost: %v", err)
	}
	encoded, marshalErr := json.Marshal(kernel.FaultJSON(err))
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var fault struct {
		Error struct {
			Code         kernel.ErrorCode `json:"code"`
			WriteOutcome string           `json:"writeOutcome"`
		} `json:"error"`
	}
	if json.Unmarshal(encoded, &fault) != nil || fault.Error.Code != kernel.ErrTemporaryUnavailable || fault.Error.WriteOutcome != "NOT_APPLIED" {
		t.Fatalf("proof lost across protocol boundary: %s", encoded)
	}
	if _, exists := ledger.Lookup("unapplied-wire"); exists {
		t.Fatal("proven unapplied command claim was not released")
	}
}
