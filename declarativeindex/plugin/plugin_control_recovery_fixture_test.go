package plugin_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	indexcontrol "kc/declarativeindex/control"
	declarative "kc/declarativeindex/plugin"
	indeximplementation "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	discoverydatasets "kc/discovery/datasets"
	consumerplugin "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

// The actual Entry owns the Index, durable Controller and Dataset recovery
// consumer. This fixture changes only the physical medium or Repository HEAD
// read; canonical values, schema interpretation and accepted releases stay real.
type controlRecoveryFixture struct {
	base      *pluginFixture
	registry  dataset.Registry
	directory string
	lookup    func(kernel.RepositoryID) (objectmodel.Repository, error)
	mu        sync.Mutex
	builds    []controlPhysicalBuild
	buildFail func(controlPhysicalBuild) error
}

type controlPhysicalBuild struct {
	Repository kernel.RepositoryID
	Meta       index.Meta
}

type controlRecoveryRuntime struct {
	plugins    *consumerplugin.IndexPlugins
	controller *indexcontrol.Controller
	queries    *indeximplementation.Index
}

func newControlRecoveryFixture(t *testing.T) *controlRecoveryFixture {
	t.Helper()
	f := &controlRecoveryFixture{base: newPluginFixture(t), directory: filepath.Join(t.TempDir(), "projection")}
	f.lookup = f.base.lookup
	sources := snapshot.NewRegistry()
	for _, source := range f.base.setups {
		if err := sources.Add(source.Repo.Snapshot()); err != nil {
			t.Fatal(err)
		}
	}
	f.registry = testkit.MakeRegistry(t)
	cat, err := dataset.NewService(sources, f.registry)
	if err != nil {
		t.Fatal(err)
	}
	testkit.RegisterMounted(t, cat, sources)
	f.base.cat = cat
	return f
}

func (f *controlRecoveryFixture) open(t *testing.T, repositories ...kernel.RepositoryID) *controlRecoveryRuntime {
	t.Helper()
	runtime := &controlRecoveryRuntime{}
	entry := declarative.Entry(declarative.Options{Directory: f.directory, Lookup: f.lookup, Datasets: f.base.cat,
		Engine: func(namespace string) index.EngineOpener {
			open := f.base.pool.opener(namespace)
			return func(directory string, id kernel.RepositoryID) (index.Engine, error) {
				physical, err := open(directory, id)
				if err != nil {
					return nil, err
				}
				return &controlRecoveryEngine{pluginPhysicalEngine: physical.(*pluginPhysicalEngine), fixture: f}, nil
			}
		},
	})
	open := entry.Open
	entry.Open = func(scope consumerplugin.PluginScope) (consumerplugin.IndexContribution, error) {
		contribution, err := open(scope)
		if err == nil {
			runtime.controller = contribution.Maintenance.(*indexcontrol.Controller)
			runtime.queries = contribution.Queries.(*indeximplementation.Index)
		}
		return contribution, err
	}
	configs := consumerplugin.RepositoryPluginConfigs{Repositories: map[kernel.RepositoryID]consumerplugin.RepositoryPluginConfig{}}
	for _, id := range repositories {
		configs.Repositories[id] = consumerplugin.RepositoryPluginConfig{Version: 1, Plugins: []consumerplugin.PluginSelection{{Name: "declarative-index", Instance: "control-recovery"}}}
	}
	var err error
	runtime.plugins, err = consumerplugin.OpenIndexPlugins(configs, map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.plugins.Close() })
	return runtime
}

func (f *controlRecoveryFixture) publish(t *testing.T, runtime *controlRecoveryRuntime, name string, id kernel.RepositoryID, at kernel.CommitID) dataset.KnowledgeSet {
	t.Helper()
	prepare := discoverydatasets.Preparation{Enabled: true, Lookup: f.lookup, Provider: runtime.plugins}
	def, err := (discoverydatasets.DatasetPublisher{Registry: f.base.cat,
		Authorize: func(context.Context, discoverydatasets.DatasetPublication) error { return nil }, Prepare: prepare.Execute,
	}).Execute(context.Background(), discoverydatasets.DatasetPublication{Dataset: name, Revision: 1,
		Sources: []dataset.KnowledgeSetSource{{Repository: id, BaseRev: string(at), Selector: snapshot.DefaultRef, Path: dataset.MountPath("content")}},
	})
	if err != nil || len(def.Sources) != 1 || def.Sources[0].Commit != at {
		t.Fatalf("actual fixed preparation/publication failed: %#v %v", def, err)
	}
	return def
}

func (f *controlRecoveryFixture) advance(t *testing.T, source int, label string) kernel.CommitID {
	t.Helper()
	s := f.base.setups[source]
	at := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
	receipt, err := s.Writer.Commit("control-source-"+label, objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/one"}, SchemaRef: "schema/article", Value: map[string]any{"body": "runbook source at " + label}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt.Result.NewCommit
}

type controlPublicationEvidence struct {
	state dataset.State
	head  string
	refs  map[kernel.RepositoryID]kernel.CommitID
}

func (f *controlRecoveryFixture) accepted(t *testing.T) controlPublicationEvidence {
	t.Helper()
	head, err := f.registry.Head()
	if err != nil {
		t.Fatal(err)
	}
	evidence := controlPublicationEvidence{state: dataset.NormalizeState(f.base.cat.DumpState()), head: head, refs: map[kernel.RepositoryID]kernel.CommitID{}}
	for _, source := range f.base.setups {
		evidence.refs[source.RepositoryID] = testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	}
	return evidence
}

func (f *controlRecoveryFixture) assertAccepted(t *testing.T, evidence controlPublicationEvidence) {
	t.Helper()
	if state := dataset.NormalizeState(f.base.cat.DumpState()); !reflect.DeepEqual(state, evidence.state) {
		t.Errorf("maintenance changed Dataset serving release/history: before=%#v after=%#v", evidence.state, state)
	}
	state, err := f.registry.Load()
	if err != nil || !reflect.DeepEqual(dataset.NormalizeState(state), evidence.state) {
		t.Errorf("maintenance changed persisted Dataset authority: %#v %v", state, err)
	}
	if head, err := f.registry.Head(); err != nil || head != evidence.head {
		t.Errorf("maintenance advanced Dataset authority: got=%s want=%s error=%v", head, evidence.head, err)
	}
	for _, source := range f.base.setups {
		if head := testkit.MustHead(t, source.Repo, snapshot.DefaultRef); head != evidence.refs[source.RepositoryID] {
			t.Errorf("maintenance changed accepted source HEAD: source=%s got=%s want=%s", source.RepositoryID, head, evidence.refs[source.RepositoryID])
		}
	}
}

func (f *controlRecoveryFixture) discardFixed(t *testing.T, id kernel.RepositoryID, at kernel.CommitID) {
	t.Helper()
	f.base.pool.mu.Lock()
	defer f.base.pool.mu.Unlock()
	discarded := 0
	for key, store := range f.base.pool.stores {
		if !strings.Contains(key, string(id)+"@") || !strings.HasSuffix(key, "@"+string(at)) {
			continue
		}
		store.mu.Lock()
		if store.meta.Basis == at {
			store.meta = index.Meta{}
			store.docs = map[objectmodel.ObjectID]index.CompiledDoc{}
			discarded++
		}
		store.mu.Unlock()
	}
	if discarded != 1 {
		t.Fatalf("fault did not discard exactly the actual fixed physical projection: source=%s basis=%s count=%d", id, at, discarded)
	}
}

func (f *controlRecoveryFixture) assertSnapshot(t *testing.T, runtime *controlRecoveryRuntime, id kernel.RepositoryID, at kernel.CommitID, body string) {
	t.Helper()
	repo, err := f.base.lookup(id)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := runtime.plugins.CheckSearchProjectionAt(repo, at)
	if err != nil || meta.Basis != at || meta.State != index.ProjectionStateReady {
		t.Errorf("fixed Dataset projection was not actually restored: basis=%s meta=%#v error=%v", at, meta, err)
		return
	}
	result, err := runtime.plugins.SearchAtContext(context.Background(), repo, at, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(result.Hits) != 1 || result.Completeness != retrieval.CompletenessComplete || result.SearchView.Snapshots[id] != at {
		t.Errorf("fixed Dataset query failed after actual recovery: %#v %v", result, err)
		return
	}
	canonical, err := repo.Read("article/one", at)
	if err != nil || canonical.Value.(map[string]any)["body"] != body || !reflect.DeepEqual(result.Hits[0].Knowledge, canonical) || !reflect.DeepEqual(result.Hits[0].Version, retrieval.VersionOf(canonical)) {
		t.Errorf("recovered candidate and complete canonical value used different bases: hit=%#v canonical=%#v error=%v", result.Hits[0], canonical, err)
	}
}

type controlRecoveryEngine struct {
	*pluginPhysicalEngine
	fixture *controlRecoveryFixture
}

func (e *controlRecoveryEngine) Rebuild(docs []index.CompiledDoc, meta index.Meta) error {
	e.fixture.mu.Lock()
	build := controlPhysicalBuild{Repository: e.repository, Meta: meta}
	e.fixture.builds = append(e.fixture.builds, build)
	var failure error
	if e.fixture.buildFail != nil {
		failure = e.fixture.buildFail(build)
	}
	e.fixture.mu.Unlock()
	if failure != nil {
		return failure
	}
	return e.pluginPhysicalEngine.Rebuild(docs, meta)
}

func (e *controlRecoveryEngine) BeginRebuild(meta index.Meta) (index.RebuildSession, error) {
	return &controlRecoverySession{engine: e, meta: meta}, nil
}

type controlRecoverySession struct {
	engine *controlRecoveryEngine
	meta   index.Meta
	docs   []index.CompiledDoc
}

func (s *controlRecoverySession) Append(docs []index.CompiledDoc) error {
	s.docs = append(s.docs, docs...)
	return nil
}
func (s *controlRecoverySession) Commit() error       { return s.engine.Rebuild(s.docs, s.meta) }
func (*controlRecoverySession) Abort(err error) error { return err }

// Preserve every real Reader capability used in projection preparation and
// same-basis hydration; only DefaultRef HEAD is made unavailable.
type controlHeadFailure struct {
	objectmodel.Repository
	objectmodel.StoreIdentified
	objectmodel.SchemaStore
	objectmodel.SchemaAdoptionLocator
	objectmodel.BindingLocator
	objectmodel.SnapshotObjectPager
	objectmodel.BatchReadStore
	objectmodel.FastObjectChanges
	err error
}

func (r controlHeadFailure) Head(ref string) (kernel.CommitID, error) {
	if snapshot.RefOrDefault(ref) == snapshot.DefaultRef {
		return "", r.err
	}
	return r.Repository.Head(ref)
}

func failingControlHead(repo objectmodel.Repository, err error) objectmodel.Repository {
	return controlHeadFailure{Repository: repo, StoreIdentified: repo.(objectmodel.StoreIdentified), SchemaStore: repo.(objectmodel.SchemaStore),
		SchemaAdoptionLocator: repo.(objectmodel.SchemaAdoptionLocator),
		BindingLocator:        repo.(objectmodel.BindingLocator), SnapshotObjectPager: repo.(objectmodel.SnapshotObjectPager),
		BatchReadStore: repo.(objectmodel.BatchReadStore), FastObjectChanges: repo.(objectmodel.FastObjectChanges), err: err}
}

type controlStateLookup struct {
	mu       sync.Mutex
	requests []liveaccess.StateLookupRequest
	status   string
	revision string
}

func (s *controlStateLookup) LookupState(_ context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	return liveaccess.StateObservation{Value: map[string]any{"status": s.status}, Basis: objectmodel.ObservationBasis{
		BindingGeneration: "control-binding-generation", Consistency: objectmodel.ObservationRepeatable, SourceRevision: s.revision, ObservedAt: "2026-10-14T00:00:00Z",
	}}, nil
}

func controlObservationSource(lookup *controlStateLookup) index.ObservationSource {
	return sourcebridge.Source{Lookup: lookup, Request: objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "index-control-worker"}, RequestID: "index-control-recovery"}}
}

func (f *controlRecoveryFixture) stateFiles(t *testing.T) int {
	t.Helper()
	count := 0
	err := filepath.Walk(f.directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.Contains(path, string(filepath.Separator)+"state-serving"+string(filepath.Separator)) && filepath.Ext(path) == ".db" {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}
