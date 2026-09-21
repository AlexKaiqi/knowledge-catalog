package catalog

func yamlFiles(state CatalogState, catalogID string) (map[string][]byte, error) {
	out := map[string][]byte{}
	body, err := encodeYAML(catalogMeta{ID: catalogID, Archived: state.Archived})
	if err != nil {
		return nil, err
	}
	out[CatalogFile()] = body
	for _, version := range state.DatasetVersions {
		b, err := encodeYAML(version)
		if err != nil {
			return nil, err
		}
		out[datasetVersionFile(version.SetID, version.Revision)] = b
	}
	for _, workspace := range state.KnowledgeSets {
		b, err := encodeYAML(workspace)
		if err != nil {
			return nil, err
		}
		out[KnowledgeSetYAML(workspace.SetID)] = b
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

func asKnowledgeSetYAML(body []byte) (KnowledgeSet, error) {
	var def KnowledgeSet
	err := decodeYAML(body, &def)
	return def, err
}
