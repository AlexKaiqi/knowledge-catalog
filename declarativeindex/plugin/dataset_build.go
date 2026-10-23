package plugin

import (
	"context"
	"sync"

	projection "kc/declarativeindex/projection"
	discoverydatasets "kc/discovery/datasets"
	"kc/kernel"
	"kc/object/reader"
	"kc/snapshot/dataset"
)

// datasetBuild owns the corpus of accepted Dataset releases in this instance.
// Acceptance happens in the publication request; this runs afterwards, so a
// large source cannot hold the publisher open (doc/design/dataset.md#发布).
type datasetBuild struct {
	idx         *projection.Index
	options     Options
	publication discoverydatasets.PublicationRecovery
	lookup      reader.MemberLookup
	// gate serializes the two entries to one build of one release. The
	// background lane and explicit preparation are the same build, not two
	// implementations; without the gate both fence the same release and the
	// later fence invalidates the earlier admission, so one entry fails with
	// NON_FAST_FORWARD for no reason the caller can act on.
	gate *buildGate
	// owns reports whether every source of a release belongs to this instance.
	// A deployment may run several instances; a release must be built by the one
	// that interprets all of its sources, never by each instance separately.
	owns func(dataset.Dataset) bool
}

// buildGate holds one lock per accepted release. Two entries may race on the
// same release in one process; distinct releases still build concurrently, and
// a gate is never held across a different release's build.
type buildGate struct {
	mu    sync.Mutex
	locks map[dataset.PublicationLookup]*sync.Mutex
}

func newBuildGate() *buildGate {
	return &buildGate{locks: map[dataset.PublicationLookup]*sync.Mutex{}}
}

func (g *buildGate) lock(key dataset.PublicationLookup) func() {
	g.mu.Lock()
	entry, ok := g.locks[key]
	if !ok {
		entry = &sync.Mutex{}
		g.locks[key] = entry
	}
	g.mu.Unlock()
	entry.Lock()
	return entry.Unlock
}

// Demand is the accepted-release view this lane builds, including the "already
// adopted and ready" test that keeps a finished build from repeating.
func (b datasetBuild) Demand() discoverydatasets.AcceptedReleaseDemand {
	return discoverydatasets.AcceptedReleaseDemand{Releases: b.options.Datasets, Ready: b.ready, Owns: b.owns, Build: b.BuildDataset}
}

// ready reports whether this exact revision already has a prepared corpus this
// release provably owns. Unadopted or failed releases stay buildable, so the
// next pass is their retry path and no separate queue is needed.
func (b datasetBuild) ready(ctx context.Context, setID string, revision int) (bool, error) {
	def, err := b.options.Datasets.DatasetVersion(setID, revision)
	if kernel.CodeOf(err) == kernel.ErrDatasetInvalid || kernel.CodeOf(err) == kernel.ErrVersionUnresolved {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	evidence, err := b.publication.Evidence(ctx, setID, revision)
	if err != nil || len(evidence) == 0 {
		// No durable preparation record: nothing proves this release owns a
		// corpus, so it must be built and adopted even if one exists locally.
		return false, err
	}
	resolved, err := b.options.Datasets.ResolveDefinition(def)
	if err != nil {
		return false, err
	}
	readiness, err := b.idx.DatasetReadiness(ctx, b.lookup, b.options.DatasetPin(resolved))
	if err != nil {
		return false, err
	}
	return readiness.Ready, nil
}

// BuildDataset builds and adopts one accepted release's corpus. It takes the
// release coordinate, never a source HEAD. Both entries pass through the same
// per-release gate, so a retry that starts while the background lane is already
// building the same release waits for it instead of fencing it again.
func (b datasetBuild) BuildDataset(ctx context.Context, def dataset.Dataset) error {
	key := dataset.PublicationLookup{SetID: def.SetID, Revision: def.Revision}
	if b.gate != nil {
		unlock, err := lockBuildGate(ctx, b.gate, key)
		if err != nil {
			return err
		}
		defer unlock()
	}
	return b.publication.Adopt(ctx, key, func(ctx context.Context, accepted dataset.Dataset) (discoverydatasets.PublicationProtection, error) {
		resolved, err := b.options.Datasets.ResolveDefinition(accepted)
		if err != nil {
			return nil, err
		}
		return b.idx.PrepareDatasetResource(ctx, b.lookup, b.options.DatasetPin(resolved))
	})
}

// lockBuildGate waits for the release lock without ignoring cancellation.
func lockBuildGate(ctx context.Context, gate *buildGate, key dataset.PublicationLookup) (func(), error) {
	if ctx == nil || ctx.Done() == nil {
		return gate.lock(key), nil
	}
	done := make(chan func(), 1)
	go func() { done <- gate.lock(key) }()
	select {
	case unlock := <-done:
		return unlock, nil
	case <-ctx.Done():
		// The waiter still holds nothing; release the lock once it is granted so
		// a cancelled caller cannot strand the release.
		go func() { (<-done)() }()
		return nil, ctx.Err()
	}
}

// buildAccepted is the explicit-preparation entry: same build, same adoption,
// addressed by the accepted release the caller named. A caller may name an
// older revision on purpose, so it resolves that release rather than the
// current one; an already adopted and ready release is answered, not rebuilt,
// because adoption is idempotent and preparation reuses a ready corpus.
func (b datasetBuild) buildAccepted(ctx context.Context, setID string, revision int) error {
	def, err := b.options.Datasets.DatasetVersion(setID, revision)
	if err != nil {
		return err
	}
	return b.BuildDataset(ctx, def)
}
