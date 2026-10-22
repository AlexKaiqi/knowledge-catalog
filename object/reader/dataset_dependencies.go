package reader

import (
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// Dataset dependencies (doc/design/dataset.md#发布): a selected binding unit
// may name a ResourceDescriptor held in a Repository the Dataset did not
// select. Publication freezes all candidate versions used to prove the unique
// holder, including absence witnesses; reads use them only to interpret the
// reference. A dependency does not grant membership, so
// Read, Holds, Contains, the file scope and search stay on pin.Repositories.

// RepositoryInventory lists the Repositories a publication may take a
// dependency from. The System Repository is excluded by the freeze itself.
type RepositoryInventory func() []kernel.RepositoryID

// FreezeDependencies resolves every descriptorRef named by a binding unit in
// the Dataset scope. A reference held exactly once in scope needs nothing; one
// held nowhere in scope is looked up by ID across the other Repositories (a
// selected Repository at its frozen commit, any other at its DefaultRef head)
// and must have exactly one holder. No holder or several holders fail the
// preparation instead of picking one. Successful external resolution retains
// the complete tested candidate vector for later replay of uniqueness.
func (s *Serving) FreezeDependencies(inventory RepositoryInventory) (map[kernel.RepositoryID]kernel.CommitID, error) {
	refs, err := s.bindingDescriptorRefs()
	if err != nil {
		return nil, err
	}
	dependencies := map[kernel.RepositoryID]kernel.CommitID{}
	// One preparation must test every reference against the same version of
	// each candidate, including candidates that did not hold an earlier ref.
	commits := make(map[kernel.RepositoryID]kernel.CommitID, len(s.pin.Repositories))
	for id, commit := range s.pin.Repositories {
		commits[id] = commit
	}
	var candidates []kernel.RepositoryID
	for _, ref := range refs {
		inScope, err := s.Read(ref)
		if err != nil && kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
			return nil, err
		}
		switch {
		case len(inScope) == 1:
			continue
		case len(inScope) > 1:
			return nil, kernel.Fail(kernel.ErrObjectIDConflict,
				"descriptorRef %s is held by several repositories in the Dataset", ref)
		}
		if candidates == nil {
			if inventory == nil {
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset publication cannot locate descriptorRef %s without a repository inventory", ref)
			}
			candidates = append([]kernel.RepositoryID(nil), inventory()...)
			sortRepoIDs(candidates)
		}
		repository, commit, err := s.dependencyHolder(ref, candidates, commits)
		if err != nil {
			return nil, err
		}
		dependencies[repository] = commit
	}
	if len(dependencies) == 0 {
		return nil, nil
	}
	// Retain every candidate tested to prove uniqueness, including sources
	// that contributed no descriptor. A later replay can then reproduce the
	// absence proof instead of rediscovering current heads.
	for id, commit := range commits {
		if id != objectmodel.SystemRepositoryID {
			dependencies[id] = commit
		}
	}
	return dependencies, nil
}

func (s *Serving) dependencyHolder(ref objectmodel.ObjectID, ids []kernel.RepositoryID, commits map[kernel.RepositoryID]kernel.CommitID) (kernel.RepositoryID, kernel.CommitID, error) {
	var holders []kernel.RepositoryID
	var holderCommit kernel.CommitID
	for _, id := range ids {
		if id == objectmodel.SystemRepositoryID {
			continue
		}
		repo, err := s.lookup(id)
		if err != nil {
			return "", "", err
		}
		commit, fixed := commits[id]
		if !fixed {
			head, err := repo.Head(snapshot.DefaultRef)
			if err != nil {
				return "", "", err
			}
			if head == "" {
				return "", "", kernel.Fail(kernel.ErrPreconditionFailed,
					"repository %s has no published head while resolving descriptorRef %s", id, ref)
			}
			commit = head
			commits[id] = commit
		}
		if _, err := repo.Read(ref, commit); err != nil {
			if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
				continue
			}
			return "", "", err
		}
		holders = append(holders, id)
		holderCommit = commit
	}
	switch len(holders) {
	case 0:
		return "", "", kernel.Fail(kernel.ErrObjectRefUnresolved,
			"descriptorRef %s is held by no repository; the Dataset revision cannot be prepared", ref)
	case 1:
		return holders[0], holderCommit, nil
	default:
		return "", "", kernel.Fail(kernel.ErrObjectIDConflict,
			"descriptorRef %s is held by several repositories %v; the Dataset revision cannot be prepared", ref, holders)
	}
}

// bindingDescriptorRefs lists, once each, the descriptorRefs named by binding
// units inside the Dataset scope, from metadata-only binding coordinates.
func (s *Serving) bindingDescriptorRefs() ([]objectmodel.ObjectID, error) {
	seen := map[objectmodel.ObjectID]bool{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo objectmodel.Repository) error {
		if !datasetHasRepository(s.pin.Items, repositoryID) {
			return nil
		}
		member, ok := repo.(*datasetRepository)
		if !ok {
			return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s is not a Dataset-scoped member", repositoryID)
		}
		references, err := member.bindingReferences()
		if err != nil {
			return err
		}
		for _, reference := range references {
			seen[reference.DescriptorRef] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	refs := make([]objectmodel.ObjectID, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i] < refs[j] })
	return refs, nil
}

// ReadDependency reads one object at the frozen dependency versions, only to
// interpret a reference made from inside the Dataset. It is not a consumer
// read: the result is never listed, searched or returned as a Dataset object.
func (s *Serving) ReadDependency(objectID objectmodel.ObjectID) ([]objectmodel.FederatedValue, error) {
	ids := make([]kernel.RepositoryID, 0, len(s.pin.Dependencies))
	for id := range s.pin.Dependencies {
		ids = append(ids, id)
	}
	sortRepoIDs(ids)
	out := []objectmodel.FederatedValue{}
	for _, id := range ids {
		commit := s.pin.Dependencies[id]
		repo, err := s.lookup(id)
		if err != nil {
			return nil, err
		}
		value, err := repo.Read(objectID, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
				continue
			}
			return nil, err
		}
		out = append(out, federatedOf(id, commit, objectID, value, value.Value))
	}
	return out, nil
}
