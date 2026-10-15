package invocation

import (
	"sort"

	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	"kc/snapshot"
)

// requestDescriptors is the descriptorRef scope of a direct Repository read:
// the read Repository at the read commit first, then every other Repository
// the caller may read at its DefaultRef head. The System Repository holds no
// ResourceDescriptors. A descriptor held by several other Repositories is
// ambiguous rather than picked by order.
func requestDescriptors(cx *Invocation, repository kernel.RepositoryID, commit kernel.CommitID) liveaccess.DescriptorScope {
	return func(id objectmodel.ObjectID) (liveaccess.DescriptorAt, error) {
		if at, found, err := descriptorAt(cx, repository, commit, id); err != nil || found {
			return at, err
		}
		ids := cx.WS.Store.IDs()
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		held := []liveaccess.DescriptorAt{}
		for _, other := range ids {
			if other == repository || other == objectmodel.SystemRepositoryID {
				continue
			}
			store, ok := cx.WS.Store.Get(other)
			if !ok {
				continue
			}
			head, err := store.Head(snapshot.DefaultRef)
			if err != nil || head == "" {
				continue
			}
			at, found, err := descriptorAt(cx, other, head, id)
			if err != nil {
				return liveaccess.DescriptorAt{}, err
			}
			if found {
				held = append(held, at)
			}
		}
		switch len(held) {
		case 0:
			return liveaccess.DescriptorAt{}, kernel.Fail(kernel.ErrObjectRefUnresolved,
				"descriptorRef %s is not readable in this request scope", id)
		case 1:
			return held[0], nil
		default:
			return liveaccess.DescriptorAt{}, kernel.Fail(kernel.ErrObjectIDConflict,
				"descriptorRef %s is held by several readable repositories", id)
		}
	}
}

func descriptorAt(cx *Invocation, repository kernel.RepositoryID, commit kernel.CommitID, id objectmodel.ObjectID) (liveaccess.DescriptorAt, bool, error) {
	if !AllowedRepoRead(cx.Home, cx.Flags, string(repository), string(id)) {
		return liveaccess.DescriptorAt{}, false, nil
	}
	repo, err := cx.WS.Reader.Require(repository, kernel.ErrObjectRefUnresolved)
	if err != nil {
		return liveaccess.DescriptorAt{}, false, err
	}
	value, err := repo.Read(id, commit)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return liveaccess.DescriptorAt{}, false, nil
	}
	if err != nil {
		return liveaccess.DescriptorAt{}, false, err
	}
	return liveaccess.DescriptorAt{Repository: repository, Commit: commit, Value: value.Value}, true, nil
}
