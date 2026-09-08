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
	view := *ws
	view.readOnly = true
	view.File = ws.BindingFile()
	view.Journal = j
	view.Reader = reader.NewReader(ws.Store)
	view.Reader.SetJournal(j)
	view.Reader.SetHydrator(ws.Hydrator)
	view.Catalogs = make(map[string]*catalog.Catalog, len(ws.Catalogs))
	for id, original := range ws.Catalogs {
		scoped := original.ReadView(j)
		view.Catalogs[id] = scoped
		if original == ws.Catalog {
			view.Catalog = scoped
		}
	}
	return &view
}
