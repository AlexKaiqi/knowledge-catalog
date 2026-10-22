package dataset

import (
	"context"
	"sync"
	"sync/atomic"

	"kc/kernel"
	"kc/snapshot"
)

// registry persists service metadata/membership, current Dataset definitions,
// and immutable releases as flat registry.yaml, dataset-*.yaml, release-*.yaml.
// The independent Snapshot authority is not an Object Repository.
//
// Reads never take a lock: the accepted head is an atomic value and every
// read addresses an immutable commit. writeMu serializes only commits of the
// registry document; the authority CAS (ExpectedTargetCommit) still guards
// other processes.
type registry struct {
	authority snapshot.Store
	tree      snapshot.TreeStore
	writeMu   sync.Mutex
	head      atomic.Pointer[string]
	ref       string
}

func (g *registry) acceptedHead() string {
	if head := g.head.Load(); head != nil {
		return *head
	}
	return ""
}

func (g *registry) accept(head string) { g.head.Store(&head) }

func (g *registry) AuthorityID() string { return string(g.authority.ID()) }

// Close releases the Snapshot authority opened for a production service.
func (g *registry) Close() error {
	if g == nil || g.authority == nil {
		return nil
	}
	if closer, ok := g.authority.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

// Head is the registry commit the current combination space was read from.
func (g *registry) Head() (string, error) {
	head := g.acceptedHead()
	if head == "" {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "service registry has no accepted commit")
	}
	return head, nil
}

// Load reads the current flat Workspace registry YAML from the accepted commit.
func (g *registry) Load() (State, error) {
	state, _, err := g.loadSnapshot()
	return state, err
}

func (g *registry) loadSnapshot() (State, string, error) {
	head := g.acceptedHead()
	state, err := g.loadAt(head)
	return state, head, err
}

func (g *registry) loadAt(head string) (State, error) {
	files, err := g.snapshotYAML(head)
	if err != nil {
		return State{}, err
	}
	if len(files) > 0 {
		return g.stateFromYAML(files)
	}
	if err := g.requireVirginAuthority(head); err != nil {
		return State{}, err
	}
	return EmptyState, nil
}

// Save publishes state against this handle's last accepted authority commit.
// A stale handle must be reopened; Save never merges or overwrites newer state.
func (g *registry) Save(state State, message, author, requestID, ruleID string) error {
	g.writeMu.Lock()
	defer g.writeMu.Unlock()
	return g.saveSnapshot(state, message, author, requestID, ruleID)
}

func (g *registry) saveExpected(expected string, state State, message, author, requestID, ruleID string) (string, error) {
	return g.saveExpectedContext(context.Background(), expected, state, message, author, requestID, ruleID)
}

func (g *registry) saveExpectedContext(ctx context.Context, expected string, state State, message, author, requestID, ruleID string) (string, error) {
	return g.saveReceiptExpectedContext(ctx, expected, state, message, author, requestID, ruleID, nil)
}

func (g *registry) saveReceiptExpectedContext(ctx context.Context, expected string, state State, message, author, requestID, ruleID string, receipt *PublicationReceipt) (string, error) {
	unlock, err := lockForContext(ctx, &g.writeMu)
	if err != nil {
		return "", err
	}
	defer unlock()
	if head := g.acceptedHead(); expected != head {
		return "", kernel.Fail(kernel.ErrNonFastForward, "service authority moved: expected %s, actual %s", expected, head)
	}
	err = g.saveSnapshotReceiptContext(ctx, state, message, author, requestID, ruleID, receipt)
	return g.acceptedHead(), err
}

// CheckAuthority verifies the accepted branch without refreshing or publishing.
// Snapshot adapters must fail closed when that authority is gone or advanced.
// The common case reads without a lock. A mismatch may be this process's own
// commit between the authority advancing and acceptance, so only then is it
// settled behind the commit lock.
func (g *registry) CheckAuthority() error {
	err := g.checkAuthority()
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		return err
	}
	g.writeMu.Lock()
	defer g.writeMu.Unlock()
	return g.checkAuthority()
}

func (g *registry) checkAuthority() error {
	accepted := g.acceptedHead()
	actual, err := g.authority.Head(g.ref)
	if err != nil {
		return err
	}
	if actual == "" {
		return kernel.Fail(kernel.ErrVersionUnresolved, "service authority ref is unavailable")
	}
	if string(actual) != accepted {
		return kernel.Fail(kernel.ErrPreconditionFailed, "service authority advanced; reopen the deployment")
	}
	return nil
}
