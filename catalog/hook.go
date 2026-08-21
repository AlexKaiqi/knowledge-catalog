package catalog

import (
	"kc/kernel"
	"kc/repository"
)

// Hook is an in-process subscriber on Catalog lifecycle.
// It is not outbound kc hook-add (scripts/HTTP; see docs/HOOKS.md) and not a gate.
// Catalog never imports index; a sidecar implements this interface.
// Failure must not roll back Catalog, Writer, or Merge (K-19 / K-21).
type Hook interface {
	// AfterSnapshot: a registered repository's snapshot advanced (COMMIT or merge).
	// Writer / Merge emit on the shared Store; Catalog subscribes at NewCatalog.
	AfterSnapshot(Snapshot) error
}

// Snapshot is a registered repository moving from → to. ObjectIDs is the changed set;
// nil means unknown (subscriber should Ensure), empty means no object edits.
type Snapshot struct {
	Repository repository.Repository
	From       kernel.CommitID
	To         kernel.CommitID
	ObjectIDs  []kernel.ObjectID
}

func (c *Catalog) AddHook(h Hook) {
	if h == nil {
		return
	}
	c.hooks = append(c.hooks, h)
}

// NotifySnapshot fans Store snapshot events out to Hook subscribers.
// No-op when the repo is not registered. Hook errors are journaled, not returned.
// Callers are Writer COMMIT and ControlPlane merge (via Store), not the CLI facade.
func (c *Catalog) NotifySnapshot(ev Snapshot) {
	if ev.Repository == nil || !c.HasRepository(ev.Repository.ID()) {
		return
	}
	c.runHooks("after-snapshot", map[string]any{
		"repositoryId": string(ev.Repository.ID()),
		"from":         string(ev.From),
		"to":           string(ev.To),
	}, func(h Hook) error {
		return h.AfterSnapshot(ev)
	})
}

func (c *Catalog) runHooks(kind string, refs map[string]any, fn func(Hook) error) {
	for _, h := range c.hooks {
		if h == nil {
			continue
		}
		if err := fn(h); err != nil {
			row := map[string]any{"hook": kind, "hookError": err.Error()}
			for k, v := range refs {
				row[k] = v
			}
			_ = c.note("hook", row, nil)
		}
	}
}
