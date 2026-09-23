package home

import (
	"kc/catalog"
	"kc/internal/journal"
	"kc/knowledge/reader"
)

// ReadView borrows the process's authority handles, command ledger, Writer,
// and Index while isolating the mutable collaborators used by a read request.
// The caller must neither Close this borrowed view nor use it for mutations.
func (ws *Home) ReadView(j journal.Journal) *Home {
	// Build the borrowed view explicitly so lifecycle locks (closeOnce,
	// controlMu) are never copied; new Home fields must be carried here
	// deliberately instead of riding along a struct copy.
	view := &Home{
		inventory:    ws.inventory,
		readOnly:     true,
		Deployment:   ws.Deployment,
		Dir:          ws.Dir,
		Store:        ws.Store,
		Commands:     ws.Commands,
		Writer:       ws.Writer,
		TreeWriter:   ws.TreeWriter,
		Catalog:      ws.Catalog,
		Catalogs:     make(map[string]*catalog.Catalog, len(ws.Catalogs)),
		Registries:   ws.Registries,
		Registry:     ws.Registry,
		ControlPlane: ws.ControlPlane,
		ControlStore: ws.ControlStore,
		File:         ws.BindingFile(),
		Control:      ws.Control,
		Controls:     ws.Controls,
		controlID:    ws.controlID,
		Journal:      j,
		Index:        ws.Index,
		Projection:   ws.Projection,
		Hydrator:     ws.Hydrator,
		ReadCache:    ws.ReadCache,
		Stores:       ws.Stores,
	}
	view.Reader = reader.NewReader(ws.Store)
	view.Reader.SetJournal(j)
	view.Reader.SetHydrator(ws.Hydrator)
	for id, original := range ws.Catalogs {
		scoped := original.ReadView(j)
		view.Catalogs[id] = scoped
		if original == ws.Catalog {
			view.Catalog = scoped
		}
	}
	return view
}
