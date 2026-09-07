package home

import "kc/internal/journal"

// SetJournal installs the request-scoped process journal on every collaborator
// that records stamps. CLI transport fills the stamp; Home only forwards it.
func (ws *Home) SetJournal(j journal.Journal) {
	if ws == nil {
		return
	}
	ws.Journal = j
	if ws.Writer != nil {
		ws.Writer.SetJournal(j)
	}
	if ws.TreeWriter != nil {
		ws.TreeWriter.SetJournal(j)
	}
	if ws.Reader != nil {
		ws.Reader.SetJournal(j)
	}
	if ws.ControlPlane != nil {
		ws.ControlPlane.SetJournal(j)
	}
	for _, cat := range ws.Catalogs {
		if cat != nil {
			cat.SetJournal(j)
		}
	}
}
