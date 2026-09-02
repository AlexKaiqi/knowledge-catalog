package cli

import (
	"kc/catalog"
	"kc/controlplane"
	"kc/gate"
	"kc/index"
	"kc/kernel"
	"kc/knowledge/reader"
)

// Index AfterSnapshot and merge Gate are wired here, after Home assembly.

func (ws *Home) wireSidecars() {
	for _, cat := range ws.Catalogs {
		ws.attachIndex(cat)
	}
	ws.attachMergeGate(ws.ControlPlane)
}

type indexHook struct {
	controller *index.Controller
	knowledge  *reader.Reader
}

// Indexing is layer ③ over ②: a member attached as a plain snapshot has nothing
// to index, so it advances without an index pass rather than failing the write.
func (h *indexHook) AfterSnapshot(ev catalog.Snapshot) error {
	repo, err := h.knowledge.Wrap(ev.Repository, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
			return nil
		}
		return err
	}
	if repo == nil {
		return nil
	}
	return h.controller.Desire(repo.ID(), ev.To)
}

func (ws *Home) attachIndex(cat *catalog.Catalog) {
	if ws.Index == nil || ws.Projection == nil || cat == nil || ws.Stores.Index == "none" {
		return
	}
	cat.AddHook(&indexHook{controller: ws.Projection, knowledge: ws.Reader})
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
