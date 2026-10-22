package snapshot_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"kc/kernel"
	"kc/snapshot"
)

func stagedLedger(t *testing.T, path string) (snapshot.CommandLedger, snapshot.StagedCommandLedger) {
	t.Helper()
	ledger, err := snapshot.NewCommandLedger(snapshot.NewBoltCommandStore(path))
	if err != nil {
		t.Fatal(err)
	}
	staged, ok := ledger.(snapshot.StagedCommandLedger)
	if !ok || !staged.SupportsStages() {
		t.Fatal("Bolt ledger lacks conditional recovery")
	}
	return ledger, staged
}

func TestStagedCommandNoChangeRecoveryFencesLateExecutorAcrossLedgers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.db")
	first, old := stagedLedger(t, path)
	second, recovery := stagedLedger(t, path)
	request := snapshot.CommandRequest{Kind: "TEST", BaseCommit: "original", ExpectedTargetCommit: "original"}
	selected, release := make(chan struct{}), make(chan struct{})
	oldDone := make(chan error, 1)
	var retained snapshot.CommandAttempt
	go func() {
		_, _, err := old.ExecuteStaged("no-change", "digest", request, func(attempt snapshot.CommandAttempt) (any, error) {
			retained = attempt
			if err := attempt.SelectNoChange(); err != nil {
				return nil, err
			}
			close(selected)
			<-release
			if err := attempt.BeginCommit(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				return nil, errors.New("no-change attempt allowed a commit")
			}
			return map[string]any{"executor": "old"}, nil
		})
		oldDone <- err
	}()
	<-selected
	entry, replayed, err := recovery.ExecuteStaged("no-change", "digest", request, func(attempt snapshot.CommandAttempt) (any, error) {
		if !attempt.NoChangeOnly() {
			t.Error("recovery lost its no-commit proof")
		}
		if err := attempt.BeginCommit(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Errorf("recovery may commit: %v", err)
		}
		if err := attempt.SelectNoChange(); err != nil {
			return nil, err
		}
		return map[string]any{"executor": "recovery"}, nil
	})
	if err != nil || replayed || entry.Status != snapshot.StatusApplied {
		t.Fatalf("recover: %#v %v %v", entry, replayed, err)
	}
	close(release)
	if err := <-oldDone; kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("late execution overwrote recovered receipt: %v", err)
	}
	if err := retained.SelectNoChange(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("closed attempt remained writable: %v", err)
	}
	for _, ledger := range []snapshot.CommandLedger{first, second} {
		actual, ok := ledger.Lookup("no-change")
		if !ok || string(actual.Receipt) != string(entry.Receipt) || actual.Execution != entry.Execution {
			t.Fatalf("stale cached command receipt: %#v want %#v", actual, entry)
		}
	}
	_, replayed, err = recovery.ExecuteStaged("no-change", "digest", request, func(snapshot.CommandAttempt) (any, error) { t.Fatal("applied receipt reran callback"); return nil, nil })
	if err != nil || !replayed {
		t.Fatalf("final receipt not replayed: %v %v", replayed, err)
	}
}

func TestStagedCommandUnknownAndCommitStartedNeverResume(t *testing.T) {
	for _, phase := range []string{"reserved", "commit", "legacy"} {
		t.Run(phase, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "commands.db")
			ledger, staged := stagedLedger(t, path)
			request := snapshot.CommandRequest{Kind: "TEST", BaseCommit: "fixed"}
			uncertain := errors.New("process stopped before receipt")
			if phase == "legacy" {
				_, _, _ = ledger.Execute("pending", "digest", request, func() (any, error) { return nil, uncertain })
			} else {
				_, _, _ = staged.ExecuteStaged("pending", "digest", request, func(attempt snapshot.CommandAttempt) (any, error) {
					if phase == "commit" {
						if err := attempt.BeginCommit(); err != nil {
							return nil, err
						}
					}
					return nil, uncertain
				})
			}
			_, cold := stagedLedger(t, path)
			_, _, err := cold.ExecuteStaged("pending", "digest", request, func(snapshot.CommandAttempt) (any, error) { t.Fatal("uncertain command was reissued"); return nil, nil })
			if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("unsafe recovery: %v", err)
			}
		})
	}
}

func TestStagedCommandRecoveryRetainsOriginalRequestAndRejectsChangedCoordinates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.db")
	ledger, staged := stagedLedger(t, path)
	request := snapshot.CommandRequest{Kind: "TEST", BaseCommit: "fixed", ExpectedTargetCommit: "fixed"}
	_, _, _ = staged.ExecuteStaged("pending", "digest", request, func(attempt snapshot.CommandAttempt) (any, error) {
		if err := attempt.SelectNoChange(); err != nil {
			return nil, err
		}
		return nil, errors.New("receipt lost")
	})
	changed := request
	changed.BaseCommit = "new-head"
	_, _, err := staged.ExecuteStaged("pending", "digest", changed, func(snapshot.CommandAttempt) (any, error) {
		t.Fatal("recovery replaced original base")
		return nil, nil
	})
	if kernel.CodeOf(err) != kernel.ErrIdempotencyConflict {
		t.Fatalf("changed coordinates accepted: %v", err)
	}
	// An explicit operator conclusion fences every older completion as well.
	if _, err := ledger.ResolvePending("pending", "digest", map[string]any{"operator": "verified"}); err != nil {
		t.Fatal(err)
	}
	_, replayed, err := staged.ExecuteStaged("pending", "digest", request, func(snapshot.CommandAttempt) (any, error) {
		t.Fatal("operator receipt reran execution")
		return nil, nil
	})
	if err != nil || !replayed {
		t.Fatalf("operator receipt was lost: %v", err)
	}
}

func TestConditionalCommandReservationRunsOneCallbackAcrossIndependentLedgers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.db")
	first, _ := stagedLedger(t, path)
	second, _ := stagedLedger(t, path)
	arrived, release := make(chan struct{}), make(chan struct{})
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		_, _, err := first.Execute("shared", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) { close(arrived); <-release; return map[string]any{"ok": true}, nil })
		if err != nil {
			t.Errorf("first: %v", err)
		}
	}()
	<-arrived
	_, _, err := second.Execute("shared", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) { t.Error("second authority call crossed an active claim"); return nil, nil })
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("concurrent claim did not fail closed: %v", err)
	}
	close(release)
	group.Wait()
	_, replayed, err := second.Execute("shared", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) { t.Fatal("cached pending entry hid completed receipt"); return nil, nil })
	if err != nil || !replayed {
		t.Fatalf("independent ledger did not see receipt: %v", err)
	}
}

type receiptCASFault struct {
	snapshot.CommandStore
	cas    snapshot.ConditionalCommandStore
	accept bool
}

func (s *receiptCASFault) CompareAndSwap(id string, expected, next *snapshot.CommandEntry) (bool, error) {
	if next != nil && next.Status == snapshot.StatusApplied {
		if s.accept {
			if _, err := s.cas.CompareAndSwap(id, expected, next); err != nil {
				return false, err
			}
		}
		return false, errors.New("receipt acknowledgement lost")
	}
	return s.cas.CompareAndSwap(id, expected, next)
}

func TestStagedCommandDistinguishesMissingFinalReceiptFromLostAcknowledgement(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "not-saved", true: "saved-response-lost"}[accepted], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "commands.db")
			store := snapshot.NewBoltCommandStore(path)
			faulty := &receiptCASFault{CommandStore: store, cas: store.(snapshot.ConditionalCommandStore), accept: accepted}
			ledger, err := snapshot.NewCommandLedger(faulty)
			if err != nil {
				t.Fatal(err)
			}
			request := snapshot.CommandRequest{Kind: "TEST", BaseCommit: "original", ExpectedTargetCommit: "original"}
			_, _, err = ledger.(snapshot.StagedCommandLedger).ExecuteStaged("no-change", "digest", request, func(attempt snapshot.CommandAttempt) (any, error) {
				if err := attempt.SelectNoChange(); err != nil {
					return nil, err
				}
				return map[string]any{"accepted": "original"}, nil
			})
			if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
				t.Fatalf("lost final acknowledgement returned success: %v", err)
			}
			_, cold := stagedLedger(t, path)
			calls := 0
			entry, replayed, err := cold.ExecuteStaged("no-change", "digest", request, func(attempt snapshot.CommandAttempt) (any, error) {
				calls++
				if !attempt.NoChangeOnly() {
					t.Error("no-change recovery allowed a fresh commit")
				}
				return map[string]any{"accepted": "new-check-at-original-base"}, nil
			})
			if err != nil || replayed != accepted || calls != map[bool]int{false: 1, true: 0}[accepted] {
				t.Fatalf("receipt recovery guessed: saved=%v calls=%d replayed=%v err=%v", accepted, calls, replayed, err)
			}
			want := `{"accepted":"new-check-at-original-base"}`
			if accepted {
				want = `{"accepted":"original"}`
			}
			if string(entry.Receipt) != want {
				t.Fatalf("receipt changed: %s want %s", entry.Receipt, want)
			}
		})
	}
}

func TestStagedCommandLateUnappliedFailureCannotDeleteRecoveredReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.db")
	first, old := stagedLedger(t, path)
	_, recovery := stagedLedger(t, path)
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	request := snapshot.CommandRequest{Kind: "TEST", BaseCommit: "fixed"}
	go func() {
		_, _, err := old.ExecuteStaged("same", "digest", request, func(attempt snapshot.CommandAttempt) (any, error) {
			if err := attempt.SelectNoChange(); err != nil {
				return nil, err
			}
			close(entered)
			<-release
			return nil, snapshot.WriteNotApplied(errors.New("old check failed"))
		})
		done <- err
	}()
	<-entered
	accepted, _, err := recovery.ExecuteStaged("same", "digest", request, func(snapshot.CommandAttempt) (any, error) { return map[string]any{"verified": true}, nil })
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !kernel.IsWriteNotApplied(err) {
		t.Fatalf("lost explicit non-application proof: %v", err)
	}
	actual, ok := first.Lookup("same")
	if !ok || actual.Status != snapshot.StatusApplied || string(actual.Receipt) != string(accepted.Receipt) {
		t.Fatalf("late cleanup erased newer receipt: %#v", actual)
	}
}

func TestStagedCommandRequiresConditionalPersistence(t *testing.T) {
	ledger, err := snapshot.NewCommandLedger(&memoryStore{})
	if err != nil {
		t.Fatal(err)
	}
	staged := ledger.(snapshot.StagedCommandLedger)
	if staged.SupportsStages() {
		t.Fatal("nonconditional persistence claimed fencing")
	}
	_, _, err = staged.ExecuteStaged("unsupported", "digest", snapshot.CommandRequest{}, func(snapshot.CommandAttempt) (any, error) {
		t.Fatal("unsafe store executed staged callback")
		return nil, nil
	})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unsupported store accepted recovery: %v", err)
	}
}

type absentReadBarrier struct {
	snapshot.CommandStore
	cas     snapshot.ConditionalCommandStore
	mu      sync.Mutex
	readers int
	both    chan struct{}
}

func (s *absentReadBarrier) Get(id string) (snapshot.CommandEntry, bool, error) {
	entry, found, err := s.CommandStore.Get(id)
	if id == "race" && !found && err == nil {
		s.mu.Lock()
		s.readers++
		count := s.readers
		if count == 2 {
			close(s.both)
		}
		s.mu.Unlock()
		if count <= 2 {
			<-s.both
		}
	}
	return entry, found, err
}
func (s *absentReadBarrier) CompareAndSwap(id string, expected, next *snapshot.CommandEntry) (bool, error) {
	return s.cas.CompareAndSwap(id, expected, next)
}

func TestConditionalCommandClaimSurvivesSimultaneousAbsentReads(t *testing.T) {
	base := snapshot.NewBoltCommandStore(filepath.Join(t.TempDir(), "commands.db"))
	store := &absentReadBarrier{CommandStore: base, cas: base.(snapshot.ConditionalCommandStore), both: make(chan struct{})}
	first, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	second, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	calls := 0
	done := make(chan error, 2)
	for _, ledger := range []snapshot.CommandLedger{first, second} {
		go func(ledger snapshot.CommandLedger) {
			_, _, err := ledger.Execute("race", "digest", snapshot.CommandRequest{Kind: "TEST"}, func() (any, error) { mu.Lock(); calls++; mu.Unlock(); return map[string]any{"ok": true}, nil })
			done <- err
		}(ledger)
	}
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-done
		if err == nil {
			successes++
		} else if kernel.CodeOf(err) == kernel.ErrPreconditionFailed {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 || calls != 1 {
		t.Fatalf("non-atomic reservation: successes=%d conflicts=%d authority-calls=%d", successes, conflicts, calls)
	}
}

func TestConditionalCommandCompletionNeverRecreatesMissingLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.db")
	_, staged := stagedLedger(t, path)
	_, _, err := staged.ExecuteStaged("possibly-committed", "digest", snapshot.CommandRequest{Kind: "TEST"}, func(attempt snapshot.CommandAttempt) (any, error) {
		if err := attempt.BeginCommit(); err != nil {
			return nil, err
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"authority": "accepted"}, nil
	})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("missing ledger completion: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("completion recreated missing command namespace: %v", err)
	}
}
