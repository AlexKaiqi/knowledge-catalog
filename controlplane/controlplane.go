package controlplane

import (
	"kc/gate"
	"kc/internal/journal"
	"kc/kernel"
	"kc/object/writer"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

type ControlPlane struct {
	store    snapshot.Registry
	writer   *writer.Writer
	datasets dataset.Service
	journal  journal.Journal

	mergeRequired func(repo kernel.RepositoryID) []string
	mergeEvidence func(basisID string) []gate.Evidence
}

func New(store snapshot.Registry, w *writer.Writer, cat dataset.Service) *ControlPlane {
	return &ControlPlane{store: store, writer: w, datasets: cat}
}

func (cp *ControlPlane) SetJournal(j journal.Journal) { cp.journal = j }

func (cp *ControlPlane) SetMergeGate(required func(repo kernel.RepositoryID) []string, evidence func(basisID string) []gate.Evidence) {
	cp.mergeRequired = required
	cp.mergeEvidence = evidence
}

func (cp *ControlPlane) note(cmd string, refs map[string]any, err error) error {
	return journal.Finish(cp.journal, journal.LayerSystem, "controlplane", cmd, refs, err)
}
