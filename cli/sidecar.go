package cli

import (
	"kc/catalog"
	"kc/controlplane"
	"kc/gate"
	"kc/index"
	"kc/kernel"
	"kc/knowledge"
)

func (ws *Home) wireSidecars() {
	for _, cat := range ws.Catalogs {
		ws.attachIndex(cat)
	}
	ws.attachMergeGate(ws.ControlPlane)
}

type indexHook struct{ idx *index.Index }

// Indexing is layer ③ over ②: a member mounted as a plain snapshot has nothing
// to index, so it advances without an index pass rather than failing the write.
func (h *indexHook) AfterSnapshot(ev catalog.Snapshot) error {
	repo, ok := knowledge.Of(ev.Repository)
	if !ok {
		return nil
	}
	return h.idx.AfterSnapshot(repo, ev.From, ev.To, nil)
}

func (ws *Home) attachIndex(cat *catalog.Catalog) {
	if ws.Index == nil || cat == nil {
		return
	}
	cat.AddHook(&indexHook{idx: ws.Index})
}

func (ws *Home) attachMergeGate(plane *controlplane.ControlPlane) {
	if plane == nil {
		return
	}
	plane.SetMergeGate(ws.mergeRequired, ws.mergeEvidence)
}

func (ws *Home) mergeRequired(repo kernel.RepositoryID) []string {
	file, err := gate.Read(ws.Dir)
	if err != nil {
		return nil
	}
	return file.Required(gate.OnMerge, string(repo), "")
}

func (ws *Home) mergeEvidence(basisID string) []gate.Evidence {
	return ws.evidenceOn(basisID, true)
}

func (ws *Home) evidenceOn(basisID string, includeStructure bool) []gate.Evidence {
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
