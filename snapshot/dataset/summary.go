package dataset

func PublicKnowledgeSets(workspaces []KnowledgeSet) []map[string]any {
	out := make([]map[string]any, 0, len(workspaces))
	for _, workspace := range workspaces {
		out = append(out, PublicKnowledgeSet(workspace))
	}
	return out
}

func PublicKnowledgeSet(workspace KnowledgeSet) map[string]any {
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
	return out
}
