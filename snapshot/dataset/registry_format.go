package dataset

import (
	"strings"

	"kc/kernel"
)

// The old per-member layout is read-compatible. Recovery never rewrites it;
// only a successful explicit mutation writes the consolidated layout.
func (g *registry) stateFromYAML(files map[string][]byte) (State, error) {
	meta, err := g.readRegistryMeta(files)
	if err != nil {
		return State{}, err
	}
	state := State{AuthorityID: g.AuthorityID(), Repositories: meta.Repositories}
	for path, body := range files {
		switch {
		case strings.HasPrefix(path, repositoryFilePrefix):
			if meta.Repositories != nil {
				return State{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "mixed service membership layouts at %s", path)
			}
			id, err := legacyRegistryMember(path, body)
			if err != nil {
				return State{}, err
			}
			state.Repositories = append(state.Repositories, id)
		case isDatasetRegistryFile(path), strings.HasPrefix(path, datasetVersionPrefix):
			def, err := asKnowledgeSetYAML(body)
			if err != nil {
				return State{}, registryFormatError(path, err)
			}
			if err := validateRegistryDefinition(path, def); err != nil {
				return State{}, err
			}
			if isDatasetRegistryFile(path) {
				state.KnowledgeSets = append(state.KnowledgeSets, def)
			} else {
				state.DatasetVersions = append(state.DatasetVersions, def)
			}
		}
	}
	if err := validateRegistryMeta(registryMeta{AuthorityID: g.AuthorityID(), Repositories: state.Repositories}); err != nil {
		return State{}, err
	}
	if err := validateRegistryReleases(state); err != nil {
		return State{}, err
	}
	return NormalizeState(state), nil
}

// Legacy metadata is read-only on recovery. A successful explicit mutation
// replaces it with registry.yaml; opening never rewrites accepted history.
func (g *registry) readRegistryMeta(files map[string][]byte) (registryMeta, error) {
	body, current := files[RegistryFile()]
	legacy, old := files["catalog.yaml"]
	if current && old {
		return registryMeta{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "mixed registry metadata layouts")
	}
	if current {
		var meta registryMeta
		if err := decodeYAML(body, &meta); err != nil {
			return registryMeta{}, registryFormatError(RegistryFile(), err)
		}
		if err := validateRegistryMeta(meta); err != nil {
			return registryMeta{}, err
		}
		if meta.AuthorityID != g.AuthorityID() {
			return registryMeta{}, kernel.Fail(kernel.ErrPreconditionFailed, "registry authority identity does not match Snapshot store")
		}
		return meta, nil
	}
	if !old {
		return registryMeta{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "registry metadata is missing")
	}
	var meta struct {
		ID           string   `json:"id"`
		Repositories []string `json:"repositories"`
		Archived     bool     `json:"archived,omitempty"`
	}
	if err := decodeYAML(legacy, &meta); err != nil {
		return registryMeta{}, registryFormatError("catalog.yaml", err)
	}
	if strings.TrimSpace(meta.ID) == "" || meta.Archived {
		return registryMeta{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "legacy registry requires nonempty identity and an active authority")
	}
	if meta.ID != g.AuthorityID() {
		return registryMeta{}, kernel.Fail(kernel.ErrPreconditionFailed, "legacy registry identity does not match Snapshot authority; an explicit migration is required")
	}
	return registryMeta{AuthorityID: g.AuthorityID(), Repositories: meta.Repositories}, nil
}

func registryFormatError(path string, err error) error {
	return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "invalid registry file %s: %v", path, err)
}

func validateRegistryMeta(meta registryMeta) error {
	if strings.TrimSpace(meta.AuthorityID) == "" {
		return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "registry.yaml requires a nonempty authorityId")
	}
	seen := map[string]bool{}
	for _, id := range meta.Repositories {
		if strings.TrimSpace(id) == "" || seen[id] {
			return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "registry.yaml requires unique nonempty repository identities")
		}
		seen[id] = true
	}
	return nil
}

func legacyRegistryMember(path string, body []byte) (string, error) {
	var row struct {
		Repository string `json:"repository,omitempty"`
		LegacyID   string `json:"repositoryId,omitempty"`
	}
	if err := decodeYAML(body, &row); err != nil {
		return "", registryFormatError(path, err)
	}
	id := row.Repository
	if id == "" {
		id = row.LegacyID
	}
	if strings.TrimSpace(id) == "" || (row.Repository != "" && row.LegacyID != "") || path != RepositoryFile(id) {
		return "", kernel.Fail(kernel.ErrKnowledgeSetInvalid, "invalid legacy repository identity at %s", path)
	}
	return id, nil
}

func validateRegistryDefinition(path string, def KnowledgeSet) error {
	if strings.TrimSpace(def.SetID) == "" || def.Revision <= 0 {
		return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "%s requires dataset identity and positive revision", path)
	}
	expected := KnowledgeSetYAML(def.SetID)
	if strings.HasPrefix(path, datasetVersionPrefix) {
		expected = datasetVersionFile(def.SetID, def.Revision)
		if def.Retired {
			return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "%s must not store current retirement policy", path)
		}
	}
	if path != expected {
		return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset identity does not match registry path %s", path)
	}
	shape := def
	shape.Retired = false // Retired Dataset records remain recoverable.
	if err := ValidateKnowledgeSet(shape); err != nil {
		return registryFormatError(path, err)
	}
	for _, source := range def.Sources {
		if strings.TrimSpace(string(source.Commit)) == "" {
			return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "%s requires frozen source commits", path)
		}
	}
	items, err := datasetItemsFromSources(def.Sources, nil)
	if err != nil {
		return registryFormatError(path, err)
	}
	if def.Items != nil && kernel.CanonicalDigest(def.Items) != kernel.CanonicalDigest(items) {
		return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "%s items disagree with frozen sources", path)
	}
	return nil
}

func validateRegistryReleases(state State) error {
	versions := map[string]map[int]KnowledgeSet{}
	latest := map[string]int{}
	for _, release := range state.DatasetVersions {
		if versions[release.SetID] == nil {
			versions[release.SetID] = map[int]KnowledgeSet{}
		}
		if _, exists := versions[release.SetID][release.Revision]; exists {
			return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "duplicate dataset release identity")
		}
		versions[release.SetID][release.Revision] = release
		latest[release.SetID] = max(latest[release.SetID], release.Revision)
	}
	for _, current := range state.KnowledgeSets {
		if current.Revision < latest[current.SetID] {
			return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset %s current version precedes an accepted release", current.SetID)
		}
		if release, ok := versions[current.SetID][current.Revision]; ok && registryDefinitionDigest(current) != registryDefinitionDigest(release) {
			return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset %s current definition disagrees with its release", current.SetID)
		}
	}
	return nil
}

func registryDefinitionDigest(def KnowledgeSet) kernel.Digest {
	def.Retired = false
	def.Items, _ = datasetItemsFromSources(def.Sources, nil)
	return kernel.CanonicalDigest(def)
}
