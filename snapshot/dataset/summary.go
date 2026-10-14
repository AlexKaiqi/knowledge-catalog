package dataset

import "maps"

func PublicDatasets(workspaces []Dataset) []map[string]any {
	out := make([]map[string]any, 0, len(workspaces))
	for _, workspace := range workspaces {
		out = append(out, PublicDataset(workspace))
	}
	return out
}

func PublicDataset(workspace Dataset) map[string]any {
	repos := make([]string, 0, len(workspace.Sources))
	seen := map[string]bool{}
	for _, src := range workspace.Sources {
		id := string(src.Repository)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		repos = append(repos, id)
	}
	out := map[string]any{
		"id":           workspace.SetID,
		"revision":     workspace.Revision,
		"repositories": repos,
		"itemCount":    len(workspace.Items),
	}
	if workspace.Retired {
		out["retired"] = true
	}
	if len(workspace.Metadata) > 0 {
		out["metadata"] = maps.Clone(workspace.Metadata)
	}
	return out
}
