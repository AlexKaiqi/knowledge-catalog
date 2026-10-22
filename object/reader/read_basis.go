package reader

import (
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
)

// ReadBasis captures this fixed Serving's complete candidate and dependency
// vector, including non-contributors. Callers freeze dependencies before using
// this method; it never discovers a replacement source or follows a head.
func (s *Serving) ReadBasis(reference objectmodel.Reference, shape string) (objectmodel.ReadBasis, error) {
	raw, err := objectmodel.FormatReference(reference)
	if err != nil {
		return objectmodel.ReadBasis{}, err
	}
	basis := objectmodel.ReadBasis{Version: 1, Reference: raw, Shape: shape, Dataset: s.pin.SetID, Revision: s.pin.Revision, Temporary: s.pin.Temporary}
	if basis.Candidates, err = s.readSources(s.pin.Repositories); err != nil {
		return objectmodel.ReadBasis{}, err
	}
	if basis.Dependencies, err = s.readSources(s.pin.Dependencies); err != nil {
		return objectmodel.ReadBasis{}, err
	}
	for _, item := range s.pin.Items {
		basis.Paths = append(basis.Paths, objectmodel.ReadPath{
			Repository: item.Repository, Kind: item.Kind,
			Prefix: strings.Trim(item.Prefix, "/"), File: strings.Trim(item.File, "/"),
		})
	}
	return basis, basis.Validate()
}

func (s *Serving) readSources(commits map[kernel.RepositoryID]kernel.CommitID) ([]objectmodel.ReadSource, error) {
	if s.lookup == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "read basis requires a repository lookup")
	}
	ids := make([]kernel.RepositoryID, 0, len(commits))
	for id := range commits {
		ids = append(ids, id)
	}
	sortRepoIDs(ids)
	sources := make([]objectmodel.ReadSource, 0, len(ids))
	for _, id := range ids {
		repo, err := s.lookup(id)
		if err != nil {
			return nil, err
		}
		if repo == nil || repo.ID() != id {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "read basis lookup changed repository identity")
		}
		identified, ok := repo.(objectmodel.StoreIdentified)
		if !ok || identified.StoreDigest() == "" {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s has no stable source binding", id)
		}
		if !repo.HasCommit(commits[id]) {
			return nil, kernel.Fail(kernel.ErrVersionUnresolved, "repository %s no longer has the fixed read commit", id)
		}
		sources = append(sources, objectmodel.ReadSource{Repository: id, Commit: commits[id], Binding: identified.StoreDigest()})
	}
	return sources, nil
}

// OpenReadBasis reopens already authorized coordinates, checking the current
// source bindings and every fixed commit before any content is read. Lookup
// must enforce current access, including the accepted Dataset's file scope.
// The resulting Serving owns no resource lifecycle and never consults heads.
func OpenReadBasis(lookup MemberLookup, basis objectmodel.ReadBasis) (*Serving, error) {
	return openReadBasis(lookup, basis, true)
}

func openReadBasis(lookup MemberLookup, basis objectmodel.ReadBasis, interpretation bool) (*Serving, error) {

	if err := basis.Validate(); err != nil {
		return nil, err
	}
	if interpretation && len(basis.UnresolvedDependencies) > 0 {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "read basis has unresolved interpretation dependencies")
	}
	if lookup == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "read basis requires a repository lookup")
	}
	// Retain the checked bindings for this request, rather than calling a
	// mutable resolver again after validation.
	repositories := map[kernel.RepositoryID]objectmodel.Repository{}
	vectors := [][]objectmodel.ReadSource{basis.Candidates}
	if interpretation {
		vectors = append(vectors, basis.Dependencies)
	}
	for _, sources := range vectors {
		for _, source := range sources {
			repo, exists := repositories[source.Repository]
			if !exists {
				var err error
				repo, err = lookup(source.Repository)
				if err != nil {
					return nil, err
				}
			}
			identified, ok := repo.(objectmodel.StoreIdentified)
			if repo == nil || repo.ID() != source.Repository || !ok || identified.StoreDigest() != source.Binding {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "read basis source binding changed for %s", source.Repository)
			}
			if !repo.HasCommit(source.Commit) {
				return nil, kernel.Fail(kernel.ErrVersionUnresolved, "read basis commit is unavailable for %s", source.Repository)
			}
			repositories[source.Repository] = repo
		}
	}
	pin := DatasetPin{SetID: basis.Dataset, Revision: basis.Revision, Temporary: basis.Temporary,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{}, Dependencies: map[kernel.RepositoryID]kernel.CommitID{}}
	for _, source := range basis.Candidates {
		pin.Repositories[source.Repository] = source.Commit
	}
	if interpretation {
		for _, source := range basis.Dependencies {
			pin.Dependencies[source.Repository] = source.Commit
		}
	}
	for _, path := range basis.Paths {
		pin.Items = append(pin.Items, DatasetItem{Repository: path.Repository, Commit: pin.Repositories[path.Repository],
			Kind: path.Kind, Prefix: path.Prefix, File: path.File})
	}
	return Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		repo, ok := repositories[id]
		if !ok {
			return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "repository %s is outside the fixed read basis", id)
		}
		return repo, nil
	}, pin), nil
}
