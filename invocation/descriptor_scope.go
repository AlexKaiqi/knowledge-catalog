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
	others := make([]kernel.RepositoryID, 0, len(s.repositories))
	for _, other := range s.repositories {
		if other != s.repository && other != objectmodel.SystemRepositoryID {
			others = append(others, other)
		}
	}
	// Uniqueness needs every readable candidate, so the probes run with a
	// bounded fan-out: a read's latency must not grow serially with the
	// number of Repositories in the deployment. Results are evaluated in
	// Repository order, so the reported error is the one a serial scan reports.
	probes := make([]descriptorProbe, len(others))
	work := make(chan int)
	var workers sync.WaitGroup
	for w := 0; w < min(descriptorScanWorkers, len(others)); w++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range work {
				probes[i] = s.probe(others[i], id)
			}
		}()
	}
	for i := range others {
		work <- i
	}
	close(work)
	workers.Wait()
	held := []liveaccess.DescriptorAt{}
	for i, probe := range probes {
		if probe.fetched {
			s.heads[others[i]] = probe.head
		}
		if probe.err != nil {
			return liveaccess.DescriptorAt{}, probe.err
		}
		if probe.found {
			held = append(held, probe.at)
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

// descriptorScanWorkers bounds the concurrent Repository probes of one
// descriptorRef resolution.
const descriptorScanWorkers = 16

type descriptorProbe struct {
	head    descriptorHead
	fetched bool
	at      liveaccess.DescriptorAt
	found   bool
	err     error
}

// probe checks one other Repository. It only reads s.heads, which is written
// after every probe of the resolution has finished.
func (s *requestDescriptorScope) probe(repository kernel.RepositoryID, id objectmodel.ObjectID) descriptorProbe {
	if !AllowedRepoRead(s.cx.Home, s.cx.Flags, string(repository), string(id)) {
		return descriptorProbe{}
	}
	head, cached := s.heads[repository]
	probe := descriptorProbe{head: head}
	if !cached {
		probe.head, probe.fetched = s.fetchHead(repository), true
	}
	if probe.head.err != nil {
		probe.err = probe.head.err
		return probe
	}
	probe.at, probe.found, probe.err = descriptorAt(s.cx, repository, probe.head.commit, id)
	return probe
}

func (s *requestDescriptorScope) fetchHead(repository kernel.RepositoryID) descriptorHead {
	store, ok := s.cx.WS.Store.Get(repository)
	if !ok {
		return descriptorHead{err: kernel.Fail(kernel.ErrObjectRefUnresolved, "Repository %s is no longer in the request scope", repository)}
	}
	commit, err := store.Head(snapshot.DefaultRef)
	if err == nil && commit == "" {
		err = kernel.Fail(kernel.ErrVersionUnresolved, "Repository %s has no published head", repository)
	}
	return descriptorHead{commit: commit, err: err}
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
