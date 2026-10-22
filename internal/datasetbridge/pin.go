// Package datasetbridge converts a Dataset service resolution into the object
// reader's consumer basis. It is the only field mapping of that shape: Home,
// shared fixtures and plugin tests call it instead of copying the fields.
//
// The conversion is pure. It reads one resolved value and writes one pin, so it
// cannot acquire a service handle, contact a source, or resolve HEAD.
package datasetbridge

import (
	"kc/kernel"
	"kc/object/reader"
	"kc/snapshot/dataset"
)

// Pin copies every field a consumer needs from one ResolveDataset result.
// Dependencies are carried because they are interpretation-only versions frozen
// with the revision: dropping them reads later Repository versions as if they
// were part of the revision. They are not members, so Items stays exactly the
// selected file scope and is never widened to a whole Repository.
func Pin(resolved dataset.ResolvedDataset) reader.DatasetPin {
	items := make([]reader.DatasetItem, 0, len(resolved.Items))
	for _, item := range resolved.Items {
		items = append(items, reader.DatasetItem{
			Target: item.Target, Repository: item.Repository, Commit: item.Commit,
			Kind: item.Kind, Prefix: item.Prefix, File: item.File,
		})
	}
	return reader.DatasetPin{
		SetID:        resolved.SetID,
		Revision:     resolved.Revision,
		Repositories: frozen(resolved.Repositories),
		Items:        items,
		Dependencies: frozen(resolved.Dependencies),
	}
}

// frozen copies a version map so a later advance of a source or dependency
// cannot rewrite coordinates this pin already froze.
func frozen(source map[kernel.RepositoryID]kernel.CommitID) map[kernel.RepositoryID]kernel.CommitID {
	if source == nil {
		return nil
	}
	copied := make(map[kernel.RepositoryID]kernel.CommitID, len(source))
	for repository, commit := range source {
		copied[repository] = commit
	}
	return copied
}
