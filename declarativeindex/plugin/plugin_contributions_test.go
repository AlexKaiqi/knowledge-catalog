package plugin_test

import (
	"context"
	"fmt"
	consumerplugin "kc/discovery/plugin"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	declarative "kc/declarativeindex/plugin"
	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/discovery/service"
	"kc/internal/testkit"
	"kc/kernel"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

type pluginFixture struct {
	setups []testkit.Setup
	reader *reader.Reader
	cat    dataset.Service
	pool   *pluginEnginePool
}

func newPluginFixture(t *testing.T) *pluginFixture {
	t.Helper()
	f := &pluginFixture{pool: &pluginEnginePool{stores: map[string]*pluginPhysicalStore{}}}
	registry := snapshot.NewRegistry()
	for n := 1; n <= 3; n++ {
		s := testkit.NewSetup(t, fmt.Sprintf("kr://plugin/source-%d", n))
		_, err := s.Writer.Commit(fmt.Sprintf("seed-source-%d", n), objectmodel.CommitChangeSet{
			TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
			Operations: []objectmodel.Operation{
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}}}},
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/one"}, SchemaRef: "schema/article", Value: map[string]any{"body": fmt.Sprintf("runbook source %d at a", n)}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := registry.Add(s.Repo.Snapshot()); err != nil {
			t.Fatal(err)
		}
		f.setups = append(f.setups, s)
	}
	f.reader = reader.NewReader(registry)
	f.cat = testkit.OpenDatasets(t, registry)
	return f
}

func (f *pluginFixture) lookup(id kernel.RepositoryID) (objectmodel.Repository, error) {
	return f.reader.Require(id, kernel.ErrObjectRefUnresolved)
}

func (f *pluginFixture) entry(t *testing.T) consumerplugin.IndexPluginEntry {
	t.Helper()
	return declarative.Entry(declarative.Options{Directory: filepath.Join(t.TempDir(), "projection"), Engine: f.pool.opener, Lookup: f.lookup, Datasets: f.cat})
}

func (f *pluginFixture) configs() consumerplugin.RepositoryPluginConfigs {
	out := map[kernel.RepositoryID]consumerplugin.RepositoryPluginConfig{}
	for n, instance := range []string{"main-a", "main-b"} {
		out[f.setups[n].RepositoryID] = consumerplugin.RepositoryPluginConfig{Version: 1, Plugins: []consumerplugin.PluginSelection{{Name: "declarative-index", Instance: instance}}}
	}
	return consumerplugin.RepositoryPluginConfigs{Repositories: out}
}

func TestIndexPluginsExecuteRealSourceContributionsAndKeepDatasetMappings(t *testing.T) {
	f := newPluginFixture(t)
	plugins, err := consumerplugin.OpenIndexPlugins(f.configs(), map[string]consumerplugin.IndexPluginEntry{"declarative-index": f.entry(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = plugins.Close() })
	before := f.pool.operations.Load()
	help, err := plugins.Help("declarative-index")
	if err != nil || help.Name != "declarative-index" || help.Summary == "" || len(help.Tasks) == 0 || f.pool.operations.Load() != before {
		t.Fatalf("explanation executed backend work or lost its registered content: %#v %v", help, err)
	}
	help.Tasks[0] = "mutated-by-reader"
	again, err := plugins.Help("declarative-index")
	if err != nil || again.Tasks[0] == help.Tasks[0] {
		t.Fatalf("caller mutated registered explanation: %#v %v", again, err)
	}
	if _, err := plugins.Help("missing"); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unknown explanation: %v", err)
	}
	query := retrieval.SearchOf(retrieval.SearchMATCH("runbook"))
	var sources []dataset.DatasetSource
	for n := 0; n < 2; n++ {
		s := f.setups[n]
		at := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
		repo, err := f.lookup(s.RepositoryID)
		if err != nil {
			t.Fatal(err)
		}
		if err := plugins.PrepareServingBasis(context.Background(), repo, at); err != nil {
			t.Fatal(err)
		}
		result, err := plugins.SearchAtContext(context.Background(), repo, at, query)
		assertPluginHit(t, result, err, s.RepositoryID, at, fmt.Sprintf("runbook source %d at a", n+1))
		sources = append(sources, dataset.DatasetSource{Repository: s.RepositoryID, BaseRev: string(at), Selector: snapshot.DefaultRef, Path: dataset.MountPath(fmt.Sprintf("source-%d", n+1))})
	}
	for _, set := range []string{"first", "second"} {
		if _, err := f.cat.DefineDataset(set, 1, sources); err != nil {
			t.Fatal(err)
		}
	}
	for _, set := range []string{"first", "second"} {
		resolved, err := f.cat.ResolveDataset(set)
		if err != nil {
			t.Fatal(err)
		}
		pin := pluginDatasetPin(resolved)
		before := f.pool.operations.Load()
		overview, err := (service.AccessDescribeExecutor{Repositories: f.lookup, Plugins: plugins}).Execute(context.Background(), service.AccessDescribeRequest{Pin: pin})
		if err != nil || len(overview.Specs) != 2 || len(overview.Plugins) != 2 || f.pool.operations.Load() != before {
			t.Fatalf("Dataset %s access explanation executed physical work or lost source contracts: %#v %v", set, overview, err)
		}
		for _, selected := range overview.Plugins {
			config := f.configs().Repositories[selected.Repository].Plugins[0]
			if selected.Name != config.Name || selected.Instance != config.Instance || selected.Commit != pin.Repositories[selected.Repository] || selected.Help != "kc plugin declarative-index help" || selected.Summary == "" || len(selected.Files) == 0 {
				t.Fatalf("Dataset %s explanation lost actual source selection/navigation: %#v", set, selected)
			}
		}
		serving := reader.Open(f.lookup, pin)
		executor := service.DatasetSearchExecutor{Authorize: func(context.Context) error { return nil }, Repositories: pluginRepositoryLookup{f}, Projection: plugins,
			Resolve: func(context.Context) (*reader.Serving, service.LogicalHydrator, error) {
				return serving, liveservice.Open(serving, nil, kernel.IdentityContext{}), nil
			},
			Deliver: func(_ context.Context, hit retrieval.ObjectHit) (retrieval.ObjectHit, error) { return hit, nil },
		}
		result, err := executor.Execute(context.Background(), query)
		if err != nil || result.Completeness != retrieval.CompletenessComplete || len(result.Hits) != 2 || !reflect.DeepEqual(result.SearchView.Snapshots, pin.Repositories) {
			t.Fatalf("Dataset %s lost source mapping/basis: %#v %v", set, result, err)
		}
		for _, hit := range result.Hits {
			repo, err := f.lookup(hit.Object.Repository)
			if err != nil {
				t.Fatal(err)
			}
			canonical, err := repo.Read("article/one", pin.Repositories[repo.ID()])
			if err != nil || !reflect.DeepEqual(hit.Object, canonical) || !reflect.DeepEqual(hit.Version, retrieval.VersionOf(canonical)) {
				t.Fatalf("Dataset %s did not return actual source canonical value: %#v %v", set, hit, err)
			}
		}
	}
	if len(f.pool.namespaces()) != 2 {
		t.Fatalf("Dataset composition constructed another plugin instance: %v", f.pool.namespaces())
	}
	for _, namespace := range f.pool.namespaces() {
		want := f.setups[0].RepositoryID
		if namespace == "declarative-index/main-b" {
			want = f.setups[1].RepositoryID
		}
		for _, repository := range f.pool.repositories(namespace) {
			if !strings.HasPrefix(string(repository), string(want)) {
				t.Fatalf("source used another instance's physical contribution: namespace=%s repository=%s", namespace, repository)
			}
		}
	}
	plain := f.setups[2]
	at := testkit.MustHead(t, plain.Repo, snapshot.DefaultRef)
	repo, err := f.lookup(plain.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	value, err := f.reader.Read(objectmodel.ObjectRef{Repository: plain.RepositoryID, Object: "article/one"}, at, nil)
	if err != nil || value.Value.(map[string]any)["body"] != "runbook source 3 at a" {
		t.Fatalf("unconfigured source lost basic reads: %#v %v", value, err)
	}
	before = f.pool.operations.Load()
	for name, err := range map[string]error{
		"query":   func() error { _, e := plugins.SearchAtContext(context.Background(), repo, at, query); return e }(),
		"prepare": plugins.PrepareServingBasis(context.Background(), repo, at),
		"sync":    func() error { _, e := plugins.EnsureAt(repo, at); return e }(),
	} {
		if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
			t.Errorf("unconfigured source %s did not fail closed: %v", name, err)
		}
	}
	if f.pool.operations.Load() != before {
		t.Fatal("unconfigured source used another instance as fallback")
	}
}

func pluginDatasetPin(resolved dataset.ResolvedDataset) reader.DatasetPin {
	pin := reader.DatasetPin{SetID: resolved.SetID, Revision: resolved.Revision, Repositories: resolved.Repositories}
	for _, item := range resolved.Items {
		pin.Items = append(pin.Items, reader.DatasetItem{Target: item.Target, Repository: item.Repository, Commit: item.Commit, Kind: item.Kind, Prefix: item.Prefix, File: item.File})
	}
	return pin
}

func TestIndexPluginsPreserveFixedBasisWhenSourceAdvancesDuringCandidateRead(t *testing.T) {
	f := newPluginFixture(t)
	plugins, err := consumerplugin.OpenIndexPlugins(f.configs(), map[string]consumerplugin.IndexPluginEntry{"declarative-index": f.entry(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = plugins.Close() })
	source := f.setups[0]
	at := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	repo, err := f.lookup(source.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if err := plugins.PrepareServingBasis(context.Background(), repo, at); err != nil {
		t.Fatal(err)
	}
	f.pool.beforeRetrieve = func() error {
		_, err := source.Writer.Commit("advance-during-plugin-read", objectmodel.CommitChangeSet{
			TargetRepository: source.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at,
			Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/one"}, SchemaRef: "schema/article", Value: map[string]any{"body": "changed source at b"}}},
		})
		return err
	}
	result, err := plugins.SearchAtContext(context.Background(), repo, at, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	assertPluginHit(t, result, err, source.RepositoryID, at, "runbook source 1 at a")
	next := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	latest, err := repo.Read("article/one", next)
	if next == at || err != nil || latest.Value.(map[string]any)["body"] != "changed source at b" {
		t.Fatalf("source did not actually advance before canonical read: %#v %v", latest, err)
	}
}

func TestIndexPluginsRejectOtherInstanceContinuationBeforeBackend(t *testing.T) {
	f := newPluginFixture(t)
	for n := 0; n < 2; n++ {
		source := f.setups[n]
		at := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
		if _, err := source.Writer.Commit(fmt.Sprintf("second-plugin-document-%d", n), objectmodel.CommitChangeSet{
			TargetRepository: source.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at,
			Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/two"}, SchemaRef: "schema/article", Value: map[string]any{"body": "runbook second page"}}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	plugins, err := consumerplugin.OpenIndexPlugins(f.configs(), map[string]consumerplugin.IndexPluginEntry{"declarative-index": f.entry(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = plugins.Close() })
	query := retrieval.SearchOf(retrieval.SearchMATCH("runbook"))
	query.Limit = 1
	var repositories []objectmodel.Repository
	var bases []kernel.CommitID
	for n := 0; n < 2; n++ {
		source := f.setups[n]
		repo, err := f.lookup(source.RepositoryID)
		if err != nil {
			t.Fatal(err)
		}
		at := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
		if err := plugins.PrepareServingBasis(context.Background(), repo, at); err != nil {
			t.Fatal(err)
		}
		repositories, bases = append(repositories, repo), append(bases, at)
	}
	first, err := plugins.SearchAtContext(context.Background(), repositories[0], bases[0], query)
	if err != nil || len(first.Hits) != 1 || first.Continuation == "" || first.Hits[0].Object.Address.ObjectID != "article/one" {
		t.Fatalf("real index did not produce a first page: %#v %v", first, err)
	}
	query.Continuation = first.Continuation
	before := f.pool.operations.Load()
	if _, err := plugins.SearchAtContext(context.Background(), repositories[1], bases[1], query); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || f.pool.operations.Load() != before {
		t.Fatalf("other instance's page entered physical backend: %v", err)
	}
	second, err := plugins.SearchAtContext(context.Background(), repositories[0], bases[0], query)
	if err != nil || len(second.Hits) != 1 || second.Hits[0].Object.Address.ObjectID != "article/two" || second.Hits[0].Object.Commit != bases[0] || second.Hits[0].Object.Value.(map[string]any)["body"] != "runbook second page" {
		t.Fatalf("own continuation lost actual canonical second page: %#v %v", second, err)
	}
}

func assertPluginHit(t *testing.T, result retrieval.SearchResult, err error, repo kernel.RepositoryID, at kernel.CommitID, body string) {
	t.Helper()
	if err != nil || result.Completeness != retrieval.CompletenessComplete || len(result.Hits) != 1 {
		t.Fatalf("actual index query: %#v %v", result, err)
	}
	hit := result.Hits[0]
	if hit.Object.Repository != repo || hit.Object.Commit != at || hit.Object.Address.ObjectID != "article/one" || hit.Object.Value.(map[string]any)["body"] != body || hit.Version.DeclarationCommit != at {
		t.Fatalf("query did not read exact source canonical value: %#v", hit)
	}
}

type pluginRepositoryLookup struct{ fixture *pluginFixture }

func (l pluginRepositoryLookup) Require(id kernel.RepositoryID, code kernel.ErrorCode) (objectmodel.Repository, error) {
	return l.fixture.reader.Require(id, code)
}

// Only the physical medium is replaceable. The real Index compiles Writer
// values and performs residual checks and same-version object reads.
type pluginPhysicalStore struct {
	mu   sync.Mutex
	meta index.Meta
	docs map[objectmodel.ObjectID]index.CompiledDoc
}
type pluginEnginePool struct {
	mu              sync.Mutex
	stores          map[string]*pluginPhysicalStore
	handles         []*pluginPhysicalEngine
	held            map[*pluginPhysicalEngine]bool
	closes          []string
	operations      atomic.Int64
	block           atomic.Bool
	blockRebuild    atomic.Bool
	entered         chan struct{}
	release         chan struct{}
	enterOnce       sync.Once
	beforeRetrieve  func() error
	rebuildFailures map[string]error
}

func (p *pluginEnginePool) opener(namespace string) index.EngineOpener {
	return func(_ string, repository kernel.RepositoryID) (index.Engine, error) {
		p.mu.Lock()
		defer p.mu.Unlock()
		key := namespace + "\x00" + string(repository)
		physical := p.stores[key]
		if physical == nil {
			physical = &pluginPhysicalStore{docs: map[objectmodel.ObjectID]index.CompiledDoc{}}
			p.stores[key] = physical
		}
		engine := &pluginPhysicalEngine{pool: p, store: physical, namespace: namespace, repository: repository}
		p.handles = append(p.handles, engine)
		return engine, nil
	}
}

func (p *pluginEnginePool) namespaces() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	set := map[string]bool{}
	for _, h := range p.handles {
		set[h.namespace] = true
	}
	var out []string
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (p *pluginEnginePool) repositories(namespace string) []kernel.RepositoryID {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []kernel.RepositoryID
	for _, h := range p.handles {
		if h.namespace == namespace {
			out = append(out, h.repository)
		}
	}
	return out
}

type pluginPhysicalEngine struct {
	pool       *pluginEnginePool
	store      *pluginPhysicalStore
	namespace  string
	repository kernel.RepositoryID
	closed     atomic.Int64
}

func (*pluginPhysicalEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) index.Capability {
	return index.Capability{Guarantee: index.GuaranteeSuperset, Coverage: 1}
}
func (*pluginPhysicalEngine) VerifyResidualMatch(text, query string, mode retrieval.MatchMode) (bool, error) {
	if mode != retrieval.MatchAllTerms {
		return false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "fixture only proves all-term matching")
	}
	words := map[string]bool{}
	for _, word := range strings.Fields(strings.ToLower(text)) {
		words[word] = true
	}
	for _, word := range strings.Fields(strings.ToLower(query)) {
		if !words[word] {
			return false, nil
		}
	}
	return true, nil
}
func (e *pluginPhysicalEngine) Retrieve(req index.RetrieveRequest) (index.CandidatePage, error) {
	return e.RetrieveContext(context.Background(), req)
}
func (e *pluginPhysicalEngine) RetrieveContext(ctx context.Context, req index.RetrieveRequest) (index.CandidatePage, error) {
	e.pool.operations.Add(1)
	e.pool.mu.Lock()
	before := e.pool.beforeRetrieve
	e.pool.beforeRetrieve = nil
	e.pool.mu.Unlock()
	if before != nil {
		if err := before(); err != nil {
			return index.CandidatePage{}, err
		}
	}
	if e.pool.block.Load() {
		if err := e.hold(ctx); err != nil {
			return index.CandidatePage{}, err
		}
	}
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	var ids []objectmodel.ObjectID
	for id := range e.store.docs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	start := 0
	if req.Continuation != "" {
		var err error
		start, err = strconv.Atoi(req.Continuation)
		if err != nil {
			return index.CandidatePage{}, err
		}
	}
	end := len(ids)
	if req.Search.Limit > 0 && start+req.Search.Limit < end {
		end = start + req.Search.Limit
	}
	page := index.CandidatePage{Exhausted: end == len(ids)}
	for _, id := range ids[start:end] {
		page.Candidates = append(page.Candidates, index.CandidateRef{Repository: req.Spec.Repository, ObjectID: id, Basis: e.store.meta.Basis})
	}
	if !page.Exhausted {
		page.Continuation = strconv.Itoa(end)
	}
	return page, nil
}
func (e *pluginPhysicalEngine) LoadMeta() (index.Meta, error) {
	e.pool.operations.Add(1)
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	return e.store.meta, nil
}
func (e *pluginPhysicalEngine) Rebuild(docs []index.CompiledDoc, meta index.Meta) error {
	e.pool.operations.Add(1)
	e.pool.mu.Lock()
	failure := e.pool.rebuildFailures[e.namespace]
	e.pool.mu.Unlock()
	if failure != nil {
		return failure
	}
	if e.pool.blockRebuild.Load() {
		if err := e.hold(context.Background()); err != nil {
			return err
		}
	}
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	e.store.docs = map[objectmodel.ObjectID]index.CompiledDoc{}
	for _, doc := range docs {
		e.store.docs[doc.ObjectID] = doc
	}
	e.store.meta = meta
	return nil
}
func (e *pluginPhysicalEngine) Apply(upserts []index.CompiledDoc, deletes []objectmodel.ObjectID, meta index.Meta) error {
	e.pool.operations.Add(1)
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	for _, id := range deletes {
		delete(e.store.docs, id)
	}
	for _, doc := range upserts {
		e.store.docs[doc.ObjectID] = doc
	}
	e.store.meta = meta
	return nil
}
func (e *pluginPhysicalEngine) Count() (int, error) {
	e.pool.operations.Add(1)
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	return len(e.store.docs), nil
}
func (e *pluginPhysicalEngine) Close() error {
	e.closed.Add(1)
	e.pool.mu.Lock()
	defer e.pool.mu.Unlock()
	e.pool.closes = append(e.pool.closes, e.namespace)
	return nil
}

func (e *pluginPhysicalEngine) hold(ctx context.Context) error {
	e.pool.mu.Lock()
	if e.pool.held == nil {
		e.pool.held = map[*pluginPhysicalEngine]bool{}
	}
	e.pool.held[e] = true
	e.pool.mu.Unlock()
	defer func() {
		e.pool.mu.Lock()
		delete(e.pool.held, e)
		e.pool.mu.Unlock()
	}()
	e.pool.enterOnce.Do(func() { close(e.pool.entered) })
	select {
	case <-e.pool.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
