package home

import (
	"kc/internal/journal"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

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
		ws.TreeWriter.SetObserver(rawWriteObserver(j))
	}
	if ws.Reader != nil {
		ws.Reader.SetJournal(j)
	}
	if ws.ControlPlane != nil {
		ws.ControlPlane.SetJournal(j)
	}
	{
		cat := ws.Datasets
		if cat != nil {
			cat.SetObserver(datasetObserver(j))
		}
	}
}

func datasetObserver(j journal.Journal) dataset.Observer {
	return func(operation string, refs map[string]any, err error) error {
		return journal.Finish(j, journal.LayerSystem, "registry", operation, refs, err)
	}
}
func rawWriteObserver(j journal.Journal) snapshot.WriteObserver {
	return func(surface string, refs map[string]any, err error) error {
		return journal.Finish(j, journal.LayerSystem, "treewriter", surface, refs, err)
	}
}
