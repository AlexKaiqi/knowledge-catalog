package plugin_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"kc/declarativeindex/internal/testfixture"
	declarative "kc/declarativeindex/plugin"
	indeximplementation "kc/declarativeindex/projection"
	consumerplugin "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

// Goal: an observation refresh is accepted only while its Dataset revision is
// current. A publication of the same Dataset that arrives while the engine is
// committing the new generation must be ordered after the acceptance, never
// between the last revision check and the generation switch.
func TestDatasetPublicationDuringObservationCommitIsOrderedAfterAcceptance(t *testing.T) {
	f := newFenceFixture(t)
	first, err := f.index.RefreshDatasetState(context.Background(), f.members, f.pin, f.source)
	if err != nil {
		t.Fatal(err)
	}
	f.lookup.Value = map[string]any{"status": "stopped"}
	f.lookup.Basis.SourceRevision = "r2"
	published := make(chan error, 1)
	var publishedInsideCommit, hookRan bool
	f.pool.setOnCommit(func() {
		hookRan = true
		go func() {
			_, err := f.datasets.DefineDataset("fence", 2, f.sources)
			published <- err
		}()
		select {
		case err := <-published:
			publishedInsideCommit = true
			published <- err
		case <-time.After(100 * time.Millisecond):
		}
	})
	accepted, err := f.index.RefreshDatasetState(context.Background(), f.members, f.pin, f.source)
	f.pool.setOnCommit(nil)
	if !hookRan {
		t.Fatal("refresh did not reach the engine commit")
	}
	if publishedInsideCommit {
		t.Fatal("publication of the refreshed Dataset landed while its observation generation was committing")
	}
	if err != nil || accepted.Revision == first.Revision {
		t.Fatalf("refresh of the then-current revision was not accepted: %#v %v", accepted, err)
	}
	select {
	case err := <-published:
		if err != nil {
			t.Fatalf("publication waiting for the acceptance failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("publication did not proceed after the acceptance")
	}
	if current, err := f.datasets.Set("fence"); err != nil || current.Revision != 2 {
		t.Fatalf("publication was not applied: %#v %v", current, err)
	}
	commits := f.pool.commitCount()
	if _, err := f.index.RefreshDatasetState(context.Background(), f.members, f.pin, f.source); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("superseded revision accepted another observation: %v", err)
	}
	if f.pool.commitCount() != commits {
		t.Fatal("rejected refresh still committed a generation")
	}
}

type fenceFixture struct {
	index    *indeximplementation.Index
	datasets dataset.Service
	members  reader.MemberLookup
	pin      reader.DatasetPin
	sources  []dataset.DatasetSource
	source   sourcebridge.Source
	lookup   *testfixture.StateLookup
	pool     *fencePool
}

func newFenceFixture(t *testing.T) fenceFixture {
	t.Helper()
	setup := testkit.NewSetup(t, "kr://fence/source")
	descriptor := bindingfixture.Descriptor("https://scheduler.example")
	descriptor["identity"] = string(objectmodel.IdentityShared)
	commit, err := setup.Writer.Commit("seed-fence", objectmodel.CommitChangeSet{
		TargetRepository: setup.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: setup.RootCommitID, ExpectedTargetCommit: setup.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/fence", AspectName: "definition"}, Value: map[string]any{"name": "fence"}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, Value: map[string]any{
				"entity": "Job", "aspect": "runtime",
				"fields": map[string]any{"status": map[string]any{"type": "string"}},
			}},
			testkit.IndexField("schema/job.runtime", "status", "keyword"),
			bindingfixture.PutDescriptor("resource/scheduler", descriptor),
			bindingfixture.PutAspect("job/fence", "runtime", "schema/job.runtime", "resource/scheduler"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := snapshot.NewRegistry()
	if err := store.Add(setup.Repo.Snapshot()); err != nil {
		t.Fatal(err)
	}
	rd := reader.NewReader(store)
	cat := testkit.OpenDatasets(t, store)
	sources := []dataset.DatasetSource{{Repository: setup.RepositoryID, Selector: snapshot.DefaultRef, BaseRev: string(commit.Result.NewCommit)}}
	if _, err := cat.DefineDataset("fence", 1, sources); err != nil {
		t.Fatal(err)
	}
	resolved, err := cat.ResolveDataset("fence")
	if err != nil {
		t.Fatal(err)
	}
	pin, members := testkit.DatasetPin(resolved), rd.Lookup(cat.Require)
	pool := &fencePool{engines: map[string]*fenceEngine{}}
	entry := declarative.Entry(declarative.Options{Directory: filepath.Join(t.TempDir(), "projection"),
		Engine: func(string) index.EngineOpener { return pool.open }, Datasets: cat,
		Lookup: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			return rd.Require(id, kernel.ErrObjectRefUnresolved)
		},
	})
	open := entry.Open
	var idx *indeximplementation.Index
	entry.Open = func(scope consumerplugin.PluginScope) (consumerplugin.IndexContribution, error) {
		contribution, err := open(scope)
		if err == nil {
			idx = contribution.Queries.(*indeximplementation.Index)
		}
		return contribution, err
	}
	installed := consumerplugin.PluginsConfig{"declarative-index": {Instances: map[string]consumerplugin.PluginInstanceConfig{"fence": {Catalog: declarative.Catalog}}}}
	plugins, err := consumerplugin.OpenIndexPlugins(installed, consumerplugin.PluginScope{Repositories: []kernel.RepositoryID{setup.RepositoryID}}, map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = plugins.Close() })
	if err := plugins.PrepareDatasetScope(context.Background(), members, pin); err != nil {
		t.Fatal(err)
	}
	lookup := &testfixture.StateLookup{Value: map[string]any{"status": "running"}, Basis: objectmodel.ObservationBasis{
		BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-10-19T00:00:00Z",
	}}
	return fenceFixture{index: idx, datasets: cat, members: members, pin: pin, sources: sources, lookup: lookup, pool: pool,
		source: sourcebridge.Source{Lookup: lookup, Request: objectmodel.RequestContext{
			Identity: kernel.IdentityContext{Principal: "fence-worker"}, RequestID: "fence-observation",
		}}}
}

type fencePool struct {
	mu       sync.Mutex
	engines  map[string]*fenceEngine
	onCommit func()
	commits  int
}

func (p *fencePool) open(directory string, repo kernel.RepositoryID) (index.Engine, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := directory + "\x00" + string(repo)
	if p.engines[key] == nil {
		p.engines[key] = &fenceEngine{StateEngine: &testfixture.StateEngine{}, pool: p}
	}
	return p.engines[key], nil
}

func (p *fencePool) setOnCommit(hook func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onCommit = hook
}

func (p *fencePool) commitCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.commits
}

type fenceEngine struct {
	*testfixture.StateEngine
	pool *fencePool
}

func (e *fenceEngine) BeginRebuild(meta index.Meta) (index.RebuildSession, error) {
	session, err := e.StateEngine.BeginRebuild(meta)
	if err != nil {
		return nil, err
	}
	return &fenceSession{RebuildSession: session, pool: e.pool}, nil
}

type fenceSession struct {
	index.RebuildSession
	pool *fencePool
}

func (s *fenceSession) Commit() error {
	s.pool.mu.Lock()
	s.pool.commits++
	hook := s.pool.onCommit
	s.pool.mu.Unlock()
	if hook != nil {
		hook()
	}
	return s.RebuildSession.Commit()
}
