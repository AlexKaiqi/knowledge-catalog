package invocation

import (
	"sort"
	"sync"

	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	"kc/snapshot"
)

// requestDescriptors is the descriptorRef scope of a direct Repository read:
// the read Repository at the read commit first, then every other Repository
// the caller may read at its DefaultRef head. The System Repository holds no
// ResourceDescriptors. A descriptor held by several other Repositories is
// ambiguous rather than picked by order. Only this request retains resolved
// declarations and the first permitted head of each Repository; observations
// and permissions are never reused by a subsequent request.
func requestDescriptors(cx *Invocation, repository kernel.RepositoryID, commit kernel.CommitID) liveaccess.DescriptorScope {
	ids := cx.WS.Store.IDs()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	scope := &requestDescriptorScope{
		cx: cx, repository: repository, commit: commit, repositories: ids,
		heads: map[kernel.RepositoryID]descriptorHead{}, resolved: map[objectmodel.ObjectID]descriptorResult{},
	}
	return scope.resolve
}

type descriptorHead struct {
	commit kernel.CommitID
	err    error
}

type descriptorResult struct {
	at  liveaccess.DescriptorAt
	err error
}

type requestDescriptorScope struct {
	mu           sync.Mutex
	cx           *Invocation
	repository   kernel.RepositoryID
	commit       kernel.CommitID
	repositories []kernel.RepositoryID
	heads        map[kernel.RepositoryID]descriptorHead
	resolved     map[objectmodel.ObjectID]descriptorResult
}

func (s *requestDescriptorScope) resolve(id objectmodel.ObjectID) (liveaccess.DescriptorAt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result, found := s.resolved[id]; found {
		return result.at, result.err
	}
	at, err := s.resolveUncached(id)
	s.resolved[id] = descriptorResult{at: at, err: err}
	return at, err
}

func (s *requestDescriptorScope) resolveUncached(id objectmodel.ObjectID) (liveaccess.DescriptorAt, error) {
	if at, found, err := descriptorAt(s.cx, s.repository, s.commit, id); err != nil || found {
		return at, err
	}
	held := []liveaccess.DescriptorAt{}
	for _, other := range s.repositories {
		if other == s.repository || other == objectmodel.SystemRepositoryID || !AllowedRepoRead(s.cx.Home, s.cx.Flags, string(other), string(id)) {
			continue
		}
		head, err := s.head(other)
		if err != nil {
			return liveaccess.DescriptorAt{}, err
		}
		at, found, err := descriptorAt(s.cx, other, head, id)
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

func (s *requestDescriptorScope) head(repository kernel.RepositoryID) (kernel.CommitID, error) {
	if head, found := s.heads[repository]; found {
		return head.commit, head.err
	}
	store, ok := s.cx.WS.Store.Get(repository)
	if !ok {
		return "", kernel.Fail(kernel.ErrObjectRefUnresolved, "Repository %s is no longer in the request scope", repository)
	}
	commit, err := store.Head(snapshot.DefaultRef)
	if err == nil && commit == "" {
		err = kernel.Fail(kernel.ErrVersionUnresolved, "Repository %s has no published head", repository)
	}
	s.heads[repository] = descriptorHead{commit: commit, err: err}
	return commit, err
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
