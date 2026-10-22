package reader

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// LocalReadBasis captures a fixed local selection without requiring its
// interpretation versions to remain available. Missing interpretation keeps
// its attempted coordinate as unresolved; it never follows another head.
func (s *Serving) LocalReadBasis(reference objectmodel.Reference, shape string) (objectmodel.ReadBasis, error) {
	local := *s
	local.pin = s.Pin()
	local.pin.Dependencies = nil
	basis, err := local.ReadBasis(reference, shape)
	if err != nil {
		return objectmodel.ReadBasis{}, err
	}
	ids := make([]kernel.RepositoryID, 0, len(s.pin.Dependencies))
	for id := range s.pin.Dependencies {
		ids = append(ids, id)
	}
	sortRepoIDs(ids)
	for _, id := range ids {
		commit := s.pin.Dependencies[id]
		repo, err := s.lookup(id)
		if err != nil {
			code := kernel.CodeOf(err)
			if code == "" {
				code = kernel.ErrVersionUnresolved
			}
			basis.UnresolvedDependencies = append(basis.UnresolvedDependencies, objectmodel.ReadDependencyFailure{Repository: id, Commit: commit, Code: code})
			continue
		}
		identified, ok := repo.(objectmodel.StoreIdentified)
		if repo == nil || repo.ID() != id || !ok || identified.StoreDigest() == "" {
			return objectmodel.ReadBasis{}, kernel.Fail(kernel.ErrPreconditionFailed, "local basis lookup changed interpretation source identity")
		}
		// A missing interpretation commit does not make local content incomplete.
		// Keep its immutable binding and coordinate; exact interpretation replay
		// separately checks availability through OpenReadBasis.
		basis.Dependencies = append(basis.Dependencies, objectmodel.ReadSource{Repository: id, Commit: commit, Binding: identified.StoreDigest()})
	}
	return basis, basis.Validate()
}
