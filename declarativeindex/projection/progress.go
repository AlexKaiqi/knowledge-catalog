package projection

import (
	"sync"

	ports "kc/discovery/projection"
	"kc/kernel"
)

// buildProgress records the objects this instance has finished compiling in
// one in-flight revision build. It is deliberately memory-only: progress is a
// live observation, not durable state, so a restarted process starts from zero
// instead of reporting a number it can no longer justify.
//
// Done counts objects, including objects a declaration excludes and which
// therefore produce no searchable document. Total counts the objects the build
// intends to visit. A finished build has Done == Total while the revision's
// objectCount (documents) is usually smaller; the two are not meant to match.
type buildProgress struct {
	mu     sync.RWMutex
	active map[kernel.RepositoryID]*catchUpCounter
}

type catchUpCounter struct {
	dataset  string
	revision int
	done     int
	total    int
}

func (p *buildProgress) begin(repository kernel.RepositoryID, dataset string, revision, total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active == nil {
		p.active = map[kernel.RepositoryID]*catchUpCounter{}
	}
	p.active[repository] = &catchUpCounter{dataset: dataset, revision: revision, total: total}
}

// advance records one finished object. Reporting is intentionally cheap and
// lossless in order: callers advance once per compiled object.
func (p *buildProgress) advance(repository kernel.RepositoryID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if counter, ok := p.active[repository]; ok {
		counter.done++
	}
}

func (p *buildProgress) finish(repository kernel.RepositoryID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.active, repository)
}

// snapshot reports every in-flight build. An empty result means this instance
// is not currently catching up, which is a normal answer rather than an error.
func (p *buildProgress) snapshot() []ports.IndexCatchUp {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]ports.IndexCatchUp, 0, len(p.active))
	for repository, counter := range p.active {
		out = append(out, ports.IndexCatchUp{
			Dataset:    counter.dataset,
			Revision:   counter.revision,
			Repository: repository,
			Done:       counter.done,
			Total:      counter.total,
		})
	}
	sortCatchUp(out)
	return out
}

// sortCatchUp orders rows by the coordinate a caller recognises first (Dataset
// and revision) and only then by corpus, so one revision's builds stay together
// and repeated reports of the same work do not shuffle between calls.
func sortCatchUp(entries []ports.IndexCatchUp) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && catchUpLess(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func catchUpLess(a, b ports.IndexCatchUp) bool {
	if a.Dataset != b.Dataset {
		return a.Dataset < b.Dataset
	}
	if a.Revision != b.Revision {
		return a.Revision < b.Revision
	}
	return a.Repository < b.Repository
}

// CatchUp reports the revision builds this instance is working on right now.
func (idx *Index) CatchUpProgress() []ports.IndexCatchUp { return idx.progress.snapshot() }
