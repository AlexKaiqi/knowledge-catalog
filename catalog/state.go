package catalog

import (
	"slices"
	"strings"
)

// CatalogState is the durable layer-① registry state. Repository contents and
// resolved Workspace pins are deliberately absent.
type CatalogState struct {
	KnowledgeSets   []KnowledgeSet `json:"datasets"`
	DatasetVersions []KnowledgeSet `json:"datasetVersions,omitempty"`
	Repositories    []string       `json:"repositories"`
	Archived        bool           `json:"archived,omitempty"`
	CatalogID       string         `json:"catalogId,omitempty"`
}

var EmptyCatalogState = CatalogState{
	KnowledgeSets: []KnowledgeSet{},
	Repositories:  []string{},
}

func (s CatalogState) IsEmpty() bool {
	return len(s.KnowledgeSets) == 0 && len(s.Repositories) == 0
}

func NormalizeCatalogState(state CatalogState) CatalogState {
	workspaces := slices.Clone(state.KnowledgeSets)
	slices.SortFunc(workspaces, func(a, b KnowledgeSet) int {
		return strings.Compare(a.SetID, b.SetID)
	})
	if workspaces == nil {
		workspaces = []KnowledgeSet{}
	}
	ids := slices.Clone(state.Repositories)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if ids == nil {
		ids = []string{}
	}
	return CatalogState{
		KnowledgeSets: workspaces, Repositories: ids, Archived: state.Archived,
		DatasetVersions: normalizeDatasetVersions(state.DatasetVersions),
		CatalogID:       state.CatalogID,
	}
}

func normalizeDatasetVersions(versions []KnowledgeSet) []KnowledgeSet {
	out := slices.Clone(versions)
	slices.SortFunc(out, func(a, b KnowledgeSet) int {
		if n := strings.Compare(a.SetID, b.SetID); n != 0 {
			return n
		}
		return a.Revision - b.Revision
	})
	return out
}
