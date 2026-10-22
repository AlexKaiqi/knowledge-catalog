package snapshot

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"kc/kernel"
	"kc/snapshot/internal/storecontract"
)

func (l *commandLedger) SupportsStages() bool {
	if l.store == nil {
		return true
	}
	_, ok := l.store.(ConditionalCommandStore)
	return ok
}

func (l *commandLedger) ExecuteStaged(id, digest string, request CommandRequest, apply func(CommandAttempt) (any, error)) (CommandEntry, bool, error) {
	if !l.SupportsStages() {
		return CommandEntry{}, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "command store cannot fence recoverable execution")
	}
	return l.executeCommand(id, digest, request, true, apply)
}

func (l *commandLedger) executeCommand(id, digest string, request CommandRequest, staged bool, apply func(CommandAttempt) (any, error)) (CommandEntry, bool, error) {
	unlock := l.lockCommand(id)
	defer unlock()
	prior, found, err := l.lookup(id)
	if err != nil {
		return CommandEntry{}, false, kernel.Fail(kernel.ErrTemporaryUnavailable, "read command %s from idempotency ledger: %v", id, err)
	}
	if found {
		if prior.Digest != digest {
			return CommandEntry{}, false, kernel.Fail(kernel.ErrIdempotencyConflict, "command %s reused with different payload", id)
		}
		if staged && prior.Phase != "" && kernel.CanonicalDigest(prior.Request) != kernel.CanonicalDigest(request) {
			return CommandEntry{}, false, kernel.Fail(kernel.ErrIdempotencyConflict, "command %s recovery changed its original request coordinates", id)
		}
		if prior.Status == StatusApplied && hasCommandReceipt(prior.Receipt) {
			return prior, true, nil
		}
		if !staged || prior.Status != StatusPending || prior.Phase != CommandPhaseNoChange || prior.Execution == "" || hasCommandReceipt(prior.Receipt) {
			return CommandEntry{}, false, kernel.Fail(kernel.ErrPreconditionFailed, "command %s has an unresolved prior outcome; inspect the target ref before retrying", id)
		}
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return CommandEntry{}, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	pending := CommandEntry{CommandID: id, Digest: digest, Status: StatusPending, CreatedAt: now, UpdatedAt: now, Request: request, Execution: hex.EncodeToString(nonce[:])}
	var expected *CommandEntry
	if found {
		expected = &prior
		pending.CreatedAt, pending.Phase = prior.CreatedAt, CommandPhaseNoChange
	} else if staged {
		pending.Phase = CommandPhaseReserved
	}
	if err := l.replaceCommand(expected, &pending); err != nil {
		return CommandEntry{}, false, commandStorageError("reserve command", id, err)
	}
	attempt := &commandAttempt{ledger: l, current: pending, noChangeOnly: found}
	defer attempt.close()
	receipt, applyErr := apply(attempt)
	current := attempt.close()
	if applyErr != nil {
		if !storecontract.IsWriteNotApplied(applyErr) {
			return CommandEntry{}, false, applyErr
		}
		if err := l.replaceCommand(&current, nil); err != nil {
			return CommandEntry{}, false, storecontract.WriteNotApplied(commandStorageError("release unapplied command", id, err))
		}
		return CommandEntry{}, false, applyErr
	}
	if staged && current.Phase != CommandPhaseNoChange && current.Phase != CommandPhaseCommit {
		return CommandEntry{}, false, kernel.Fail(kernel.ErrPreconditionFailed, "command %s omitted its authority boundary", id)
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return CommandEntry{}, false, err
	}
	if !hasCommandReceipt(raw) {
		return CommandEntry{}, false, kernel.Fail(kernel.ErrPreconditionFailed, "command %s apply omitted its receipt", id)
	}
	completed := current
	completed.Status, completed.Receipt, completed.UpdatedAt = StatusApplied, raw, time.Now().UTC().Format(time.RFC3339Nano)
	if err := l.replaceCommand(&current, &completed); err != nil {
		return CommandEntry{}, false, commandStorageError("persist command receipt", id, err)
	}
	return completed, false, nil
}

func commandStorageError(operation, id string, err error) error {
	if kernel.CodeOf(err) == kernel.ErrPreconditionFailed {
		return err
	}
	return kernel.Fail(kernel.ErrTemporaryUnavailable, "%s %s in idempotency ledger: %v", operation, id, err)
}

type commandAttempt struct {
	mu           sync.Mutex
	ledger       *commandLedger
	current      CommandEntry
	noChangeOnly bool
	closed       bool
}

func (a *commandAttempt) NoChangeOnly() bool    { return a.noChangeOnly }
func (a *commandAttempt) SelectNoChange() error { return a.transition(CommandPhaseNoChange) }
func (a *commandAttempt) BeginCommit() error    { return a.transition(CommandPhaseCommit) }

func (a *commandAttempt) transition(phase string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return kernel.Fail(kernel.ErrPreconditionFailed, "command attempt is closed")
	}
	if a.current.Phase == phase {
		return nil
	}
	if a.current.Phase != CommandPhaseReserved || a.noChangeOnly {
		return kernel.Fail(kernel.ErrPreconditionFailed, "command attempt cannot cross its recorded authority boundary")
	}
	next := a.current
	next.Phase, next.UpdatedAt = phase, time.Now().UTC().Format(time.RFC3339Nano)
	if err := a.ledger.replaceCommand(&a.current, &next); err != nil {
		return commandStorageError("record command phase", next.CommandID, err)
	}
	a.current = next
	return nil
}

func (a *commandAttempt) close() CommandEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	return a.current
}
