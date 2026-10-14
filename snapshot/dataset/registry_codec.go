package dataset

import (
	"strings"

	"kc/kernel"
)

func yamlFiles(state State, authorityID string) (map[string][]byte, error) {
	out := map[string][]byte{}
	body, err := encodeYAML(registryMeta{AuthorityID: authorityID, Repositories: state.Repositories})
	if err != nil {
		return nil, err
	}
	out[RegistryFile()] = body
	for _, version := range state.DatasetVersions {
		b, err := encodeYAML(version)
		if err != nil {
			return nil, err
		}
		if err := addRegistryDefinition(out, datasetVersionFile(version.SetID, version.Revision), version, b); err != nil {
			return nil, err
		}
	}
	for _, workspace := range state.Datasets {
		b, err := encodeYAML(workspace)
		if err != nil {
			return nil, err
		}
		if err := addRegistryDefinition(out, DatasetYAML(workspace.SetID), workspace, b); err != nil {
			return nil, err
		}
	}
	if err := validateRegistryMeta(registryMeta{AuthorityID: authorityID, Repositories: state.Repositories}); err != nil {
		return nil, err
	}
	if err := validateRegistryReleases(state); err != nil {
		return nil, err
	}
	return out, nil
}

func addRegistryDefinition(files map[string][]byte, path string, def Dataset, body []byte) error {
	if _, exists := files[path]; exists {
		return kernel.Fail(kernel.ErrDatasetInvalid, "registry identities collide at %s", path)
	}
	if err := validateRegistryDefinition(path, def); err != nil {
		return err
	}
	files[path] = body
	return nil
}

func asDatasetYAML(body []byte) (Dataset, error) {
	var def Dataset
	err := decodeYAML(body, &def)
	return def, err
}

// Metadata migration must not rewrite an unchanged accepted definition or
// manufacture a Dataset history entry through serialization differences.
func preserveDefinitionBytes(existing, desired map[string][]byte) error {
	for path, body := range existing {
		if !isDatasetRegistryFile(path) && !strings.HasPrefix(path, datasetVersionPrefix) {
			continue
		}
		next, retained := desired[path]
		if !retained {
			if strings.HasPrefix(path, datasetVersionPrefix) {
				return kernel.Fail(kernel.ErrNonFastForward, "accepted dataset release %s cannot be removed", path)
			}
			continue
		}
		before, err := asDatasetYAML(body)
		if err != nil {
			return registryFormatError(path, err)
		}
		after, err := asDatasetYAML(next)
		if err != nil {
			return registryFormatError(path, err)
		}
		if kernel.CanonicalDigest(before) == kernel.CanonicalDigest(after) {
			desired[path] = body
		} else if strings.HasPrefix(path, datasetVersionPrefix) {
			return kernel.Fail(kernel.ErrNonFastForward, "accepted dataset release %s cannot be rewritten", path)
		} else if before.Revision == after.Revision && registryDefinitionDigest(before) != registryDefinitionDigest(after) {
			return kernel.Fail(kernel.ErrNonFastForward, "accepted dataset revision at %s cannot be rewritten", path)
		}
	}
	return nil
}
