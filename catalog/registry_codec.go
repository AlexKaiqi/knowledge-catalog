package catalog

func yamlFiles(state CatalogState, catalogID string) (map[string][]byte, error) {
	out := map[string][]byte{}
	body, err := encodeYAML(catalogMeta{ID: catalogID, Archived: state.Archived})
	if err != nil {
		return nil, err
	}
	out[CatalogFile()] = body
	for _, workspace := range state.Workspaces {
		b, err := encodeYAML(workspace)
		if err != nil {
			return nil, err
		}
		out[WorkspaceYAML(workspace.WorkspaceID)] = b
	}
	for _, id := range state.Repositories {
		b, err := encodeYAML(map[string]string{"repository": id})
		if err != nil {
			return nil, err
		}
		out[RepositoryFile(id)] = b
	}
	return out, nil
}

func asWorkspaceDefinitionYAML(body []byte) (WorkspaceDefinition, error) {
	var def WorkspaceDefinition
	err := decodeYAML(body, &def)
	return def, err
}
