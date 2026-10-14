package dataset

import (
	"sync"

	"kc/kernel"
	"kc/snapshot"
)

// registry persists service metadata/membership, current Dataset definitions,
// and immutable releases as flat registry.yaml, dataset-*.yaml, release-*.yaml.
// The independent Snapshot authority is not an Object Repository.
type registry struct {
	authority snapshot.Store
	tree      snapshot.TreeStore
	mu        sync.Mutex
	head      string
	ref       string
}

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
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.head == "" {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "service registry has no accepted commit")
	}
	return g.head, nil
}

// Load reads the current flat Workspace registry YAML from the accepted commit.
func (g *registry) Load() (State, error) {
	state, _, err := g.loadSnapshot()
	return state, err
}

func (g *registry) loadSnapshot() (State, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	state, err := g.load()
	return state, g.head, err
}

func (g *registry) load() (State, error) {
	files, err := g.snapshotYAML()
	if err != nil {
		return State{}, err
	}
	if len(files) > 0 {
		return g.stateFromYAML(files)
	}
	if err := g.requireVirginAuthority(); err != nil {
		return State{}, err
	}
	return EmptyState, nil
}

// Save publishes state against this handle's last accepted authority commit.
// A stale handle must be reopened; Save never merges or overwrites newer state.
func (g *registry) Save(state State, message, author, requestID, ruleID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.saveSnapshot(state, message, author, requestID, ruleID)
}

func (g *registry) saveExpected(expected string, state State, message, author, requestID, ruleID string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if expected != g.head {
		return "", kernel.Fail(kernel.ErrNonFastForward, "service authority moved: expected %s, actual %s", expected, g.head)
	}
	err := g.saveSnapshot(state, message, author, requestID, ruleID)
	return g.head, err
}

// CheckAuthority verifies the accepted branch without refreshing or publishing.
// Snapshot adapters must fail closed when that authority is gone or advanced.
func (g *registry) CheckAuthority() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	actual, err := g.authority.Head(g.ref)
	if err != nil {
		return err
	}
	if actual == "" {
		return kernel.Fail(kernel.ErrVersionUnresolved, "service authority ref is unavailable")
	}
	if string(actual) != g.head {
		return kernel.Fail(kernel.ErrPreconditionFailed, "service authority advanced; reopen the deployment")
	}
	return nil
}
