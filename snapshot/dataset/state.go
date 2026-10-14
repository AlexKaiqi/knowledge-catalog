package dataset

import (
	"slices"
	"strings"
)

// State is the durable file-composition registry state. Repository contents and
// resolved Workspace pins are deliberately absent.
type State struct {
	Datasets        []Dataset `json:"datasets"`
	DatasetVersions []Dataset `json:"datasetVersions,omitempty"`
	Repositories    []string  `json:"repositories"`
	AuthorityID     string    `json:"authorityId,omitempty"`
}

var EmptyState = State{
	Datasets:     []Dataset{},
	Repositories: []string{},
}

func (s State) IsEmpty() bool {
	return len(s.Datasets) == 0 && len(s.Repositories) == 0
}

func NormalizeState(state State) State {
	workspaces := slices.Clone(state.Datasets)
	for i := range workspaces {
		workspaces[i] = cloneDataset(workspaces[i])
	}
	slices.SortFunc(workspaces, func(a, b Dataset) int {
		return strings.Compare(a.SetID, b.SetID)
	})
	if workspaces == nil {
		workspaces = []Dataset{}
	}
	ids := slices.Clone(state.Repositories)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if ids == nil {
		ids = []string{}
	}
	return State{
		Datasets: workspaces, Repositories: ids,
		DatasetVersions: normalizeDatasetVersions(state.DatasetVersions),
		AuthorityID:     state.AuthorityID,
	}
}

func normalizeDatasetVersions(versions []Dataset) []Dataset {
	out := slices.Clone(versions)
	for i := range out {
		out[i] = cloneDataset(out[i])
	}
	slices.SortFunc(out, func(a, b Dataset) int {
		if n := strings.Compare(a.SetID, b.SetID); n != 0 {
			return n
		}
		return a.Revision - b.Revision
	})
	return out
}
