package dataset

import (
	"strings"

	"kc/kernel"
)

// ValidateDataset checks the shape of a consumer definition
// without opening a Repository, resolving a selector, or persisting anything.
// Replaying an existing pin must validate its recipe without following HEAD.
func ValidateDataset(def Dataset) error {
	if def.Retired {
		return kernel.Fail(kernel.ErrDatasetInvalid, "workspace %s is retired", def.SetID)
	}
	if len(def.Sources) == 0 {
		return kernel.Fail(kernel.ErrDatasetInvalid, "a workspace must contain at least one repository")
	}
	for _, source := range def.Sources {
		if strings.TrimSpace(string(source.Repository)) == "" || strings.TrimSpace(source.Selector) == "" {
			return kernel.Fail(kernel.ErrDatasetInvalid, "a workspace source requires repository and selector")
		}
	}
	if err := validateMountPaths(def.Sources); err != nil {
		return err
	}
	return validateSourceCoordinates(def.Sources)
}

// validateDependencies keeps a revision's interpretation coordinates honest:
// each names an attached Repository at an existing commit, and a Repository
// that is also a source cannot be interpreted at a second version.
func (c *service) validateDependencies(sources []DatasetSource, dependencies map[kernel.RepositoryID]kernel.CommitID) error {
	frozen := map[kernel.RepositoryID]kernel.CommitID{}
	for _, src := range sources {
		frozen[src.Repository] = src.Commit
	}
	for repository, commit := range dependencies {
		if strings.TrimSpace(string(repository)) == "" || commit == "" {
			return kernel.Fail(kernel.ErrDatasetInvalid, "dataset dependency requires a repository and a fixed commit")
		}
		if source, selected := frozen[repository]; selected && source != commit {
			return kernel.Fail(kernel.ErrDatasetInvalid, "dataset dependency %s disagrees with its frozen source commit", repository)
		}
		if c.store == nil {
			return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "service has no snapshot store")
		}
		repo, ok := c.store.Get(repository)
		if !ok {
			return kernel.Fail(kernel.ErrDatasetInvalid, "dataset dependency %s is not attached", repository)
		}
		if !repo.HasCommit(commit) {
			return kernel.Fail(kernel.ErrVersionUnresolved, "dataset dependency commit %s does not exist in %s", commit, repository)
		}
	}
	return nil
}
