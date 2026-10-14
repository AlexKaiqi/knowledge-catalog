package projection

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// SetHydrator installs the application-owned Snapshot read port. Dynamic State
// searches continue reading their published Serving State revision.
func (idx *Index) SetHydrator(h objectmodel.Hydrator) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.hydrator = h
}

func (idx *Index) hydrateMany(repo objectmodel.Repository, commit kernel.CommitID, ids []objectmodel.ObjectID) (map[objectmodel.ObjectID]objectmodel.ObjectValue, error) {
	idx.mu.Lock()
	h := idx.hydrator
	idx.mu.Unlock()
	if h == nil {
		return hydrateMany(repo, commit, ids)
	}
	values, err := h.ReadMany(repo, commit, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if value, ok := values[id]; ok {
			if err := objectmodel.ValidateHydratedObject(repo.ID(), commit, id, value); err != nil {
				return nil, err
			}
		}
	}
	return values, nil
}
