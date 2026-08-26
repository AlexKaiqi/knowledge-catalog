package controlplane

import (
	"kc/catalog"
	"kc/gate"
	"kc/internal/journal"
	"kc/kernel"
	"kc/knowledge/writer"
	"kc/snapshot"
)

type ControlPlane struct {
	store   *snapshot.Registry
	writer  *writer.Writer
	catalog *catalog.Catalog
	journal journal.Journal

	mergeRequired func(repo kernel.RepositoryID) []string
	mergeEvidence func(basisID string) []gate.Evidence
}

func New(store *snapshot.Registry, w *writer.Writer, cat *catalog.Catalog) *ControlPlane {
	return &ControlPlane{store: store, writer: w, catalog: cat}
}

func (cp *ControlPlane) SetJournal(j journal.Journal) { cp.journal = j }

func (cp *ControlPlane) SetMergeGate(required func(repo kernel.RepositoryID) []string, evidence func(basisID string) []gate.Evidence) {
	cp.mergeRequired = required
	cp.mergeEvidence = evidence
}

func (cp *ControlPlane) note(cmd string, refs map[string]any, err error) error {
	return journal.Finish(cp.journal, journal.LayerSystem, "controlplane", cmd, refs, err)
}
