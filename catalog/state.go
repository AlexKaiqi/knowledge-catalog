package catalog

import (
	"slices"
	"strings"
)

// CatalogState is the durable layer-① registry state. Repository contents and
// resolved Workspace pins are deliberately absent.
type CatalogState struct {
	Workspaces   []WorkspaceDefinition `json:"workspaces"`
	Repositories []string              `json:"repositories"`
	Archived     bool                  `json:"archived,omitempty"`
	CatalogID    string                `json:"catalogId,omitempty"`
}

var EmptyCatalogState = CatalogState{
	Workspaces:   []WorkspaceDefinition{},
	Repositories: []string{},
}

func (s CatalogState) IsEmpty() bool {
	return len(s.Workspaces) == 0 && len(s.Repositories) == 0
}

func NormalizeCatalogState(state CatalogState) CatalogState {
	workspaces := slices.Clone(state.Workspaces)
	slices.SortFunc(workspaces, func(a, b WorkspaceDefinition) int {
		return strings.Compare(a.WorkspaceID, b.WorkspaceID)
	})
	if workspaces == nil {
		workspaces = []WorkspaceDefinition{}
	}
	ids := slices.Clone(state.Repositories)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if ids == nil {
		ids = []string{}
	}
	return CatalogState{
		Workspaces: workspaces, Repositories: ids, Archived: state.Archived,
		CatalogID: state.CatalogID,
	}
}
