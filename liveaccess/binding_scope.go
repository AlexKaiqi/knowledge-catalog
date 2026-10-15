package liveaccess

import (
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// ResolveBinding interprets the permitted declarations at the accepted source
// versions. Descriptors resolve inside the same fixed scope. It performs no
// runtime access; unavailable member targets are omitted.
func ResolveBinding(scope *reader.Serving, address objectmodel.Address) ([]ResolvedBinding, error) {
	pin := scope.Pin()
	ids := make([]kernel.RepositoryID, 0, len(pin.Repositories))
	for id := range pin.Repositories {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := []ResolvedBinding{}
	for _, id := range ids {
		binding, err := ResolveBindingAt(scope, id, address)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied || kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
				continue
			}
			return nil, err
		}
		out = append(out, binding)
	}
	return out, nil
}

// ResolveBindingAt uses the generic Object member scope for the declaration
// and the same scope for its ResourceDescriptor.
func ResolveBindingAt(scope *reader.Serving, repository kernel.RepositoryID, address objectmodel.Address) (ResolvedBinding, error) {
	commit, ok := scope.Pin().Repositories[repository]
	if !ok {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "repository %s is not in dataset", repository)
	}
	allowed, err := scope.Contains(repository, address.ObjectID)
	if err != nil {
		return ResolvedBinding{}, err
	}
	if !allowed {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "address %s is not in dataset", address.ObjectID)
	}
	repo, err := scope.Member(repository)
	if err != nil {
		return ResolvedBinding{}, err
	}
	return ResolveRepoBinding(repo, commit, address, ServingDescriptors(scope))
}
