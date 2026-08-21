package cli

import (
	"kc/catalog"
	"kc/controlplane"
	"kc/gate"
	"kc/index"
	"kc/kernel"
)

func (ws *OpenWorkspace) wireSidecars() {
	for _, cat := range ws.Catalogs {
		ws.attachIndex(cat)
	}
	ws.attachMergeGate(ws.ControlPlane)
}

type indexHook struct{ idx *index.Index }

func (h *indexHook) AfterSnapshot(ev catalog.Snapshot) error {
	return h.idx.AfterSnapshot(ev.Repository, ev.From, ev.To, ev.ObjectIDs)
}

func (ws *OpenWorkspace) attachIndex(cat *catalog.Catalog) {
	if ws.Index == nil || cat == nil {
		return
	}
	cat.AddHook(&indexHook{idx: ws.Index})
}

func (ws *OpenWorkspace) attachMergeGate(plane *controlplane.ControlPlane) {
	if plane == nil {
		return
	}
	plane.SetMergeGate(ws.mergeRequired, ws.mergeEvidence)
}

func (ws *OpenWorkspace) mergeRequired(repo kernel.RepositoryID) []string {
	file, err := gate.Read(ws.Home)
	if err != nil {
		return nil
	}
	return file.Required(gate.OnMerge, string(repo), "", "")
}

func (ws *OpenWorkspace) mergeEvidence(basisID string) []gate.Evidence {
	return ws.evidenceOn(basisID, true)
}

func (ws *OpenWorkspace) evidenceOn(basisID string, includeStructure bool) []gate.Evidence {
	out := []gate.Evidence{}
	for _, st := range ws.Controls {
		for _, report := range st.Validations {
			if report.PreviewID != basisID {
				continue
			}
			if !includeStructure && (report.SuiteRevision == gate.StructureSuite || report.SuiteRevision == gate.RequireValidate) {
				continue
			}
			out = append(out, gate.Evidence{
				Name:    report.SuiteRevision,
				BasisID: basisID,
				Outcome: report.Outcome,
			})
		}
	}
	return out
}
