package commandlog_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"kc/kernel"
	"kc/snapshot/commandlog"
)

type memoryStore struct {
	entries []commandlog.Entry
	saves   int
	failAt  int
}

func (s *memoryStore) Load() ([]commandlog.Entry, error) {
	return append([]commandlog.Entry(nil), s.entries...), nil
}

func (s *memoryStore) Save(entries []commandlog.Entry) error {
	s.saves++
	if s.saves == s.failAt {
		return errors.New("disk unavailable")
	}
	s.entries = append([]commandlog.Entry(nil), entries...)
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
