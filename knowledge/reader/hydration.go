package reader

import (
	"kc/kernel"
	"kc/knowledge"
)

// SetHydrator installs an application-owned exact-basis read port. Reader owns
// neither its cache nor its lifecycle; nil preserves direct authority reads.
func (r *Reader) SetHydrator(h knowledge.Hydrator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hydrator = h
}

func (r *Reader) hydration() knowledge.Hydrator {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hydrator
}

// SetHydrator configures this request's Serving before it is used.
func (s *Serving) SetHydrator(h knowledge.Hydrator) { s.hydrator = h }

func readHydrated(h knowledge.Hydrator, repo knowledge.Repository, id knowledge.ObjectID, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	if h == nil {
		return repo.Read(id, commit)
	}
	values, err := h.ReadMany(repo, commit, []knowledge.ObjectID{id})
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	value, ok := values[id]
	if !ok {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", id, commit)
	}
	if err := knowledge.ValidateHydratedObject(repo.ID(), commit, id, value); err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	return value, nil
}

func readAddressHydrated(h knowledge.Hydrator, repo knowledge.Repository, address knowledge.Address, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	if h == nil {
		return repo.ReadAddress(address, commit)
	}
	value, err := h.ReadAddress(repo, commit, address)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	if err := knowledge.ValidateHydratedAddress(repo.ID(), commit, address, value); err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	return value, nil
}
