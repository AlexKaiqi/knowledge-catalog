package home

import (
	"kc/internal/journal"
	"kc/object/reader"
)

// ReadView borrows the process's authority handles, command ledger, Writer,
// and Index while isolating the mutable collaborators used by a read request.
// The caller must neither Close this borrowed view nor use it for mutations.
func (ws *Home) ReadView(j journal.Journal) *Home {
	view := ws.borrow(j)
	view.readOnly = true
	if ws.Datasets != nil {
		view.Datasets = ws.Datasets.ReadView(datasetObserver(j))
	}
	return view
}

// RequestView serves one mutating request on the shared process Home. The
// authorities, command ledger, Registry state, inventory and plugins are
// shared, so every accepted write is immediately visible to other requests;
// the journal, Writer, TreeWriter, Reader and Dataset service stamps belong to
// this request alone. Concurrent requests therefore need no process-wide lock:
// versioned writes are decided by expected-commit checks, the command ledger
// serializes one command ID, the Dataset registry serializes its own document,
// and local policy files serialize their read-modify-write.
// The caller must not Close this borrowed view.
func (ws *Home) RequestView(j journal.Journal, as, requestID, ruleID string) *Home {
	view := ws.borrow(j)
	view.readOnly = ws.readOnly
	view.Writer = ws.Writer.ForRequest(j, as, requestID, ruleID)
	if ws.TreeWriter != nil {
		view.TreeWriter = ws.TreeWriter.ForRequest(rawWriteObserver(j), as, requestID, ruleID)
	}
	if ws.Datasets != nil {
		view.Datasets = ws.Datasets.RequestView(datasetObserver(j), as, requestID, ruleID)
	}
	return view
}

// borrow builds a request view explicitly so lifecycle state (closeOnce) is
// never copied; new Home fields must be carried here deliberately instead of
// riding along a struct copy.
func (ws *Home) borrow(j journal.Journal) *Home {
	view := &Home{
		inventory:          ws.inventory,
		borrowed:           true,
		Deployment:         ws.Deployment,
		Dir:                ws.Dir,
		Store:              ws.Store,
		Commands:           ws.Commands,
		Writer:             ws.Writer,
		TreeWriter:         ws.TreeWriter,
		Datasets:           ws.Datasets,
		Registry:           ws.Registry,
		File:               ws.BindingFile(),
		Journal:            j,
		Index:              ws.Index,
		Plugins:            ws.Plugins,
		LiveAccess:         ws.LiveAccess,
		ObjectMaintenance:  ws.ObjectMaintenance,
		DatasetPreparation: ws.DatasetPreparation,
		Hydrator:           ws.Hydrator,
		ReadCache:          ws.ReadCache,
		ReadBases:          ws.ReadBases,
		HistoryCheckpoints: ws.HistoryCheckpoints,
		Stores:             ws.Stores,
	}
	view.Reader = reader.NewReader(ws.Store)
	view.Reader.SetJournal(j)
	view.Reader.SetHydrator(ws.Hydrator)
	return view
}
