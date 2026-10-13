package dataset

import (
	"slices"
	"strings"
)

// State is the durable file-composition registry state. Repository contents and
// resolved Workspace pins are deliberately absent.
type State struct {
	KnowledgeSets   []KnowledgeSet `json:"datasets"`
	DatasetVersions []KnowledgeSet `json:"datasetVersions,omitempty"`
	Repositories    []string       `json:"repositories"`
	AuthorityID     string         `json:"authorityId,omitempty"`
}

var EmptyState = State{
	KnowledgeSets: []KnowledgeSet{},
	Repositories:  []string{},
}

func (s State) IsEmpty() bool {
	return len(s.KnowledgeSets) == 0 && len(s.Repositories) == 0
}

func NormalizeState(state State) State {
	workspaces := slices.Clone(state.KnowledgeSets)
	for i := range workspaces {
		workspaces[i] = cloneKnowledgeSet(workspaces[i])
	}
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
	return State{
		KnowledgeSets: workspaces, Repositories: ids,
		DatasetVersions: normalizeDatasetVersions(state.DatasetVersions),
		AuthorityID:     state.AuthorityID,
	}
}

func normalizeDatasetVersions(versions []KnowledgeSet) []KnowledgeSet {
	out := slices.Clone(versions)
	for i := range out {
		out[i] = cloneKnowledgeSet(out[i])
	}
	slices.SortFunc(out, func(a, b KnowledgeSet) int {
		if n := strings.Compare(a.SetID, b.SetID); n != 0 {
			return n
		}
		return a.Revision - b.Revision
	})
	return out
}
