package invocation

import (
	"sort"
	"sync"

	"kc/kernel"
	objectmodel "kc/object"
)

// objectReadBasis belongs to one actual application invocation, rather than
// one Aspect callback. It freezes interpretation candidates (including absent
// holders) across the independent reads a shape or composed object performs.
type objectReadBasis struct {
	fixed *objectmodel.ReadBasis
	heads sync.Map // RepositoryID -> descriptorHead
}

func (b *objectReadBasis) descriptorHead(id kernel.RepositoryID, fetch func() descriptorHead) descriptorHead {
	if b.fixed != nil {
		for _, source := range b.fixed.Dependencies {
			if source.Repository == id {
				return descriptorHead{commit: source.Commit}
			}
		}
		return descriptorHead{err: kernel.Fail(kernel.ErrVersionUnresolved, "descriptor repository is outside the fixed interpretation basis")}
	}
	if previous, ok := b.heads.Load(id); ok {
		return previous.(descriptorHead)
	}
	current := fetch()
	previous, _ := b.heads.LoadOrStore(id, current)
	return previous.(descriptorHead)
}

func (b *objectReadBasis) dependencies() (map[kernel.RepositoryID]kernel.CommitID, []objectmodel.ReadDependencyFailure) {
	commits := map[kernel.RepositoryID]kernel.CommitID{}
	failures := []objectmodel.ReadDependencyFailure{}
	b.heads.Range(func(key, value any) bool {
		head := value.(descriptorHead)
		if head.err != nil || head.commit == "" {
			code := kernel.CodeOf(head.err)
			if code == "" {
				code = kernel.ErrVersionUnresolved
			}
			failures = append(failures, objectmodel.ReadDependencyFailure{Repository: key.(kernel.RepositoryID), Code: code})
			return true
		}
		commits[key.(kernel.RepositoryID)] = head.commit
		return true
	})
	sort.Slice(failures, func(i, j int) bool { return failures[i].Repository < failures[j].Repository })
	return commits, failures
}
