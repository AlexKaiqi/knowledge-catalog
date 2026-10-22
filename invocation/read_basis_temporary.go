package invocation

import (
	"encoding/json"
	"fmt"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

// Restore only the object's original file selection, not the original virtual
// delivery layout. Object references do not name virtual target paths. The
// synthesized recipe and pin use immutable coordinates, and pass through the
// existing temporary-definition authorization and validation on every replay.
func restoreTemporaryReadBasis(cx *Invocation, basis objectmodel.ReadBasis) error {
	def := dataset.Dataset{SetID: basis.Dataset, Revision: basis.Revision, Dependencies: map[kernel.RepositoryID]kernel.CommitID{}}
	commits := map[kernel.RepositoryID]kernel.CommitID{}
	for _, source := range basis.Candidates {
		commits[source.Repository] = source.Commit
	}
	for _, source := range basis.Dependencies {
		def.Dependencies[source.Repository] = source.Commit
	}
	selected := map[kernel.RepositoryID]bool{}
	for index, path := range basis.Paths {
		target := fmt.Sprintf("basis/%09d", index)
		source := dataset.DatasetSource{Repository: path.Repository, Selector: snapshot.DefaultRef, Commit: commits[path.Repository]}
		if path.Kind == "file" {
			source.File, source.Target = path.File, target
		} else {
			source.Path, source.SubPath = dataset.MountPath(target), path.Prefix
		}
		def.Sources = append(def.Sources, source)
		selected[path.Repository] = true
	}
	if len(selected) != len(commits) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "temporary read basis does not retain every candidate's file selection")
	}
	pin, err := dataset.ReplayDefinition(def, dataset.ResolvedDataset{Repositories: commits})
	if err != nil {
		return err
	}
	raw, err := json.Marshal(pin)
	if err != nil {
		return err
	}
	cx.Flags[WorkspaceDefinitionFlag], cx.Flags["pin"] = &def, string(raw)
	return nil
}
