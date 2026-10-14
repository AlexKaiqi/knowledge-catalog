package reader

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// SetHydrator installs an application-owned exact-basis read port. Reader owns
// neither its cache nor its lifecycle; nil preserves direct authority reads.
func (r *Reader) SetHydrator(h objectmodel.Hydrator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hydrator = h
}

func (r *Reader) hydration() objectmodel.Hydrator {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hydrator
}

// SetHydrator configures this request's Serving before it is used.
func (s *Serving) SetHydrator(h objectmodel.Hydrator) { s.hydrator = h }

func readHydrated(h objectmodel.Hydrator, repo objectmodel.Repository, id objectmodel.ObjectID, commit kernel.CommitID) (objectmodel.ObjectValue, error) {
	if h == nil {
		return repo.Read(id, commit)
	}
	if err := checkHydrationScope(repo, id, commit); err != nil {
		return objectmodel.ObjectValue{}, err
	}
	values, err := h.ReadMany(repo, commit, []objectmodel.ObjectID{id})
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	value, ok := values[id]
	if !ok {
		return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "object %s is missing at commit %s", id, commit)
	}
	if err := objectmodel.ValidateHydratedObject(repo.ID(), commit, id, value); err != nil {
		return objectmodel.ObjectValue{}, err
	}
	return value, nil
}

func readAddressHydrated(h objectmodel.Hydrator, repo objectmodel.Repository, address objectmodel.Address, commit kernel.CommitID) (objectmodel.ObjectValue, error) {
	if h == nil {
		return repo.ReadAddress(address, commit)
	}
	if err := checkHydrationScope(repo, address.ObjectID, commit); err != nil {
		if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
			return objectmodel.ObjectValue{}, err
		}
		// A partial object cannot consume an entry retained for a whole Repo.
		// The existing scoped Address path may still admit exactly this unit.
		return repo.ReadAddress(address, commit)
	}
	value, err := h.ReadAddress(repo, commit, address)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	if err := objectmodel.ValidateHydratedAddress(repo.ID(), commit, address, value); err != nil {
		return objectmodel.ObjectValue{}, err
	}
	return value, nil
}

// A whole-Repo Serving can wrap an already scoped Member. Every Reader-owned
// boundary must admit the complete object before a shared cache is consulted.
func checkHydrationScope(repo objectmodel.Repository, id objectmodel.ObjectID, commit kernel.CommitID) error {
	for {
		scoped, ok := repo.(*datasetRepository)
		if !ok {
			return nil
		}
		if err := scoped.check(id, commit); err != nil {
			return err
		}
		repo = scoped.Repository
	}
}
