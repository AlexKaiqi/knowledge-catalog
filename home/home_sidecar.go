package home

import (
	"errors"
	indexplugins "kc/discovery/plugin"
	"kc/kernel"
	"kc/object/maintenance"
	"kc/object/reader"
	"kc/snapshot/dataset"
)

// Index AfterSnapshot is wired here, after Home assembly.

func (ws *Home) wireSidecars() {
	ws.attachIndex(ws.Datasets)
}

type indexHook struct {
	controller *maintenance.Coordinator
	plugins    func() *indexplugins.IndexPlugins
	object     *reader.Reader
}

// Indexing is layer ③ over ②: a member attached as a plain snapshot has nothing
// to index, so it advances without an index pass rather than failing the write.
func (h *indexHook) AfterSnapshot(ev dataset.Snapshot) error {
	repo, err := h.object.Wrap(ev.Repository, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
			return nil
		}
		return err
	}
	if repo == nil {
		return nil
	}
	var cacheErr error
	if h.controller != nil {
		cacheErr = h.controller.Desire(repo.ID(), ev.To)
	}
	return errors.Join(cacheErr, h.plugins().AfterSnapshot(ev))
}

func (ws *Home) attachIndex(cat dataset.Service) {
	if (ws.ObjectMaintenance == nil && ws.Plugins == nil) || cat == nil {
		return
	}
	cat.AddHook(&indexHook{controller: ws.ObjectMaintenance, plugins: func() *indexplugins.IndexPlugins { return ws.Plugins }, object: ws.Reader})
}
