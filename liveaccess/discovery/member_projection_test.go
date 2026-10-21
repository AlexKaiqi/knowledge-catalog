package discovery_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"kc/declarativeindex/opensearch"
	indexplugin "kc/declarativeindex/plugin"
	retrieval "kc/discovery"
	common "kc/discovery/plugin"
	ports "kc/discovery/projection"
	discoveryservice "kc/discovery/service"
	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	sourcebridge "kc/liveaccess/discovery"
	accessplugin "kc/liveaccess/plugin"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

type memberRuntimeRequest struct {
	Binding struct {
		Repository        kernel.RepositoryID `json:"repository"`
		DeclarationCommit kernel.CommitID     `json:"declarationCommit"`
		Address           objectmodel.Address `json:"address"`
		DeclarationDigest kernel.Digest       `json:"declarationDigest"`
	} `json:"binding"`
	SchemaRef string `json:"schemaRef"`
	Identity  struct {
		Principal string `json:"principal"`
		RequestID string `json:"requestId"`
	} `json:"identity"`
}

func TestDatasetSearchRechecksSharedCandidateAgainstCurrentObservation(t *testing.T) {
	f := newMemberProjectionFixture(t, "valid")
	published, err := f.index.RefreshState(context.Background(), f.repo, f.commit, f.source)
	if err != nil {
		t.Fatal(err)
	}
	f.origin.mu.Lock()
	f.origin.value = float64(47)
	f.origin.mu.Unlock()
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return f.reader.Require(id, kernel.ErrObjectRefUnresolved)
	}
	serving := reader.Open(lookup, f.pin)
	executor := discoveryservice.DatasetSearchExecutor{Authorize: func(context.Context) error { return nil }, Repositories: f.reader, Projection: f.index,
		Resolve: func(context.Context) (*reader.Serving, discoveryservice.LogicalHydrator, error) {
			return serving, liveservice.OpenRequest(serving, f.access, objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "consumer"}, RequestID: "candidate-recheck"}), nil
		},
		Deliver: func(_ context.Context, hit retrieval.ObjectHit) (retrieval.ObjectHit, error) { return hit, nil },
	}
	result, err := executor.Execute(context.Background(), memberQuery("31"))
	if err != nil || result.Completeness != retrieval.CompletenessComplete || len(result.Hits) != 0 || len(f.origin.calls()) != 2 {
		t.Fatalf("stale external candidate survived current READ predicate: %#v calls=%d %v", result, len(f.origin.calls()), err)
	}
	retained, err := f.index.SearchDataset(context.Background(), lookup, f.pin, memberQuery("31"))
	if err != nil {
		t.Fatal(err)
	}
	assertMemberObservation(t, f, retained, published.Revision)
	if len(f.origin.calls()) != 2 {
		t.Fatal("consumer recheck mutated the retained index observation")
	}
}

func TestDatasetRetainedRevisionIsNeverRefreshed(t *testing.T) {
	f := newMemberProjectionFixture(t, "valid")
	old, err := f.index.RefreshState(context.Background(), f.repo, f.commit, f.source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.datasets.DefineDataset(f.pin.SetID, 2, []dataset.DatasetSource{{Repository: f.repo.ID(), Selector: snapshot.DefaultRef, BaseRev: string(f.commit)}}); err != nil {
		t.Fatal(err)
	}
	f.origin.mu.Lock()
	f.origin.value = float64(47)
	f.origin.mu.Unlock()
	before := len(f.origin.calls())
	if _, err := f.index.RefreshState(context.Background(), f.repo, f.commit, f.source); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || len(f.origin.calls()) != before {
		t.Fatalf("retained Dataset revision refreshed external values: calls=%d %v", len(f.origin.calls()), err)
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return f.reader.Require(id, kernel.ErrObjectRefUnresolved)
	}
	current := f.pin
	current.Revision = 2
	if err := f.index.PrepareDatasetScope(context.Background(), lookup, current); err != nil {
		t.Fatal(err)
	}
	member, err := f.index.DatasetMember(lookup, current, f.repo.ID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.index.RefreshState(context.Background(), member, f.commit, f.source); err != nil {
		t.Fatal(err)
	}
	latest, err := f.index.SearchDataset(context.Background(), lookup, current, memberQuery("47"))
	if err != nil || len(latest.Hits) != 1 {
		t.Fatalf("current Dataset observation did not advance: %#v %v", latest, err)
	}
	retained, err := f.index.SearchDataset(context.Background(), lookup, f.pin, memberQuery("31"))
	if err != nil {
		t.Fatal(err)
	}
	assertMemberObservation(t, f, retained, old.Revision)
}

func TestDatasetRevisionPublishedDuringObservationPreservesOldGeneration(t *testing.T) {
	f := newMemberProjectionFixture(t, "valid")
	old, err := f.index.RefreshState(context.Background(), f.repo, f.commit, f.source)
	if err != nil {
		t.Fatal(err)
	}
	f.origin.mu.Lock()
	f.origin.value = float64(47)
	f.origin.mu.Unlock()
	source := &revisionPublishingSource{Source: f.source, publish: func() error {
		_, err := f.datasets.DefineDataset(f.pin.SetID, 2, []dataset.DatasetSource{{Repository: f.repo.ID(), Selector: snapshot.DefaultRef, BaseRev: string(f.commit)}})
		return err
	}}
	if _, err := f.index.RefreshState(context.Background(), f.repo, f.commit, source); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("revision superseded during observation was published: %v", err)
	}
	if !source.published || len(f.origin.calls()) != 2 {
		t.Fatalf("publication did not occur after candidate observation: published=%v calls=%d", source.published, len(f.origin.calls()))
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return f.reader.Require(id, kernel.ErrObjectRefUnresolved)
	}
	retained, err := f.index.SearchDataset(context.Background(), lookup, f.pin, memberQuery("31"))
	if err != nil {
		t.Fatal(err)
	}
	assertMemberObservation(t, f, retained, old.Revision)
	replacement, err := f.index.SearchDataset(context.Background(), lookup, f.pin, memberQuery("47"))
	if err != nil || len(replacement.Hits) != 0 || replacement.SearchView.ProjectionRevisions[f.repo.ID()] != old.Revision {
		t.Fatalf("rejected refresh changed retained generation or values: %#v %v", replacement, err)
	}
}

func TestDatasetCurrentRevisionObservationRefreshPublishesNewGeneration(t *testing.T) {
	f := newMemberProjectionFixture(t, "valid")
	old, err := f.index.RefreshState(context.Background(), f.repo, f.commit, f.source)
	if err != nil {
		t.Fatal(err)
	}
	f.origin.mu.Lock()
	f.origin.value = float64(47)
	f.origin.mu.Unlock()
	next, err := f.index.RefreshState(context.Background(), f.repo, f.commit, f.source)
	if err != nil || next.Revision == old.Revision || len(f.origin.calls()) != 2 {
		t.Fatalf("same Dataset revision failed to accept a new observation: %#v calls=%d %v", next, len(f.origin.calls()), err)
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return f.reader.Require(id, kernel.ErrObjectRefUnresolved)
	}
	current, err := f.index.SearchDataset(context.Background(), lookup, f.pin, memberQuery("47"))
	if err != nil || len(current.Hits) != 1 || current.SearchView.ProjectionRevisions[f.repo.ID()] != next.Revision {
		t.Fatalf("new observation generation was not queryable: %#v %v", current, err)
	}
	stale, err := f.index.SearchDataset(context.Background(), lookup, f.pin, memberQuery("31"))
	if err != nil || len(stale.Hits) != 0 || stale.SearchView.ProjectionRevisions[f.repo.ID()] != next.Revision {
		t.Fatalf("replaced observation generation remained current: %#v %v", stale, err)
	}
}

type revisionPublishingSource struct {
	sourcebridge.Source
	publish   func() error
	published bool
}

func (s *revisionPublishingSource) ObserveDatasetValue(ctx context.Context, scope *reader.Serving, repo objectmodel.Repository, value objectmodel.ObjectValue) (objectmodel.ObservedValue, error) {
	observed, err := s.Source.ObserveDatasetValue(ctx, scope, repo, value)
	if err == nil && !s.published && len(observed.Observations) > 0 {
		err = s.publish()
		s.published = err == nil
	}
	return observed, err
}

type memberOrigin struct {
	mu       sync.Mutex
	value    any
	mode     string
	requests []memberRuntimeRequest
}

func (s *memberOrigin) serve(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodPost || r.URL.Path != "/v1/access" {
		t.Errorf("unexpected source operation: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected source operation", http.StatusBadRequest)
		return
	}
	var request memberRuntimeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Error(err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.requests = append(s.requests, request)
	value, mode := s.value, s.mode
	s.mu.Unlock()
	if mode == "source" {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": kernel.IngressError{Code: kernel.ErrTemporaryUnavailable, Message: "source unavailable"}})
		return
	}
	basis := objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable,
		SourceRevision: "r31", ObservedAt: "2026-10-14T00:00:00Z"}
	if value == float64(47) {
		basis.BindingGeneration, basis.SourceRevision = "g2", "r47"
	}
	if mode == "basis" {
		basis = objectmodel.ObservationBasis{}
	}
	if mode == "schema" {
		value = "wrong number type"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"value": value}, "basis": basis})
}

func (s *memberOrigin) calls() []memberRuntimeRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]memberRuntimeRequest(nil), s.requests...)
}

type memberProjectionFixture struct {
	repo     objectmodel.Repository
	reader   *reader.Reader
	commit   kernel.CommitID
	address  objectmodel.Address
	access   *liveservice.Plugins
	index    *common.IndexPlugins
	source   sourcebridge.Source
	pin      reader.DatasetPin
	datasets dataset.Service
	origin   *memberOrigin
}

func newMemberProjectionFixture(t *testing.T, mode string) memberProjectionFixture {
	t.Helper()
	endpoint := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if endpoint == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL must name an existing OpenSearch environment")
	}
	origin := &memberOrigin{value: float64(31), mode: mode}
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { origin.serve(t, w, r) }))
	t.Cleanup(runtime.Close)
	namespace := "liveaccess-u20-" + string(kernel.CanonicalDigest(t.TempDir()))[:16]
	raw := testkit.MakeTreeStore(t, "kr://liveaccess/"+namespace)
	registry := snapshot.NewRegistry()
	if err := registry.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	address := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "sensor/one", AspectName: "readings", MemberKey: "primary"}
	descriptor := bindingfixture.Descriptor(runtime.URL)
	descriptor["identity"] = "shared"
	operations := []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/sensor.readings"}, Value: map[string]any{
			"entity": "Sensor", "aspect": "readings", "pattern": "keyed_collection",
			"fields": map[string]any{"value": map[string]any{"type": "number", "required": true}},
		}},
		testkit.IndexField("schema/sensor.readings", "value", "number", "filter", "sort"),
		bindingfixture.PutDescriptor("resource/sensor-readings", descriptor),
		bindingfixture.Bind(address, "schema/sensor.readings", "resource/sensor-readings"),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: address.ObjectID, AspectName: "definition"}, Value: map[string]any{"owner": "fixed"}},
	}
	if mode == "mixed" {
		// The same object also carries a unit whose descriptor states no
		// identity, so it is per-caller and may never enter the index.
		operations = append(operations,
			objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/sensor.private"}, Value: map[string]any{
				"entity": "Sensor", "aspect": "private", "pattern": "keyed_collection",
				"fields": map[string]any{"value": map[string]any{"type": "number", "required": true}},
			}},
			testkit.IndexField("schema/sensor.private", "value", "number", "filter", "sort"),
			bindingfixture.PutDescriptor("resource/sensor-private", bindingfixture.Descriptor(runtime.URL)),
			bindingfixture.Bind(privateMemberAddress, "schema/sensor.private", "resource/sensor-private"),
		)
	}
	receipt, err := w.Commit("publish-live-member", objectmodel.ChangeSet{TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root, Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	rd := reader.NewReader(registry)
	repo, err := rd.Require(raw.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenDatasets(t, registry)
	if _, err := cat.DefineDataset("sensor-readings", 1, []dataset.DatasetSource{{Repository: repo.ID(), Selector: snapshot.DefaultRef, BaseRev: string(receipt.Result.NewCommit)}}); err != nil {
		t.Fatal(err)
	}
	config := common.PluginsConfig{
		"liveaccess":        {Instances: map[string]common.PluginInstanceConfig{"main": {Protocol: objectmodel.ProtocolResourceAccessV1}}},
		"declarative-index": {Instances: map[string]common.PluginInstanceConfig{"main": {Catalog: indexplugin.Catalog}}},
	}
	if mode == "unselected-source" {
		delete(config, "liveaccess")
	}
	repositories := []kernel.RepositoryID{repo.ID()}
	access, err := liveservice.OpenPlugins(config, repositories,
		map[string]liveservice.PluginEntry{"liveaccess": accessplugin.Entry(accessplugin.Options{})})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = access.Close() })
	entry := indexplugin.Entry(indexplugin.Options{Directory: t.TempDir(), Lookup: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return rd.Require(id, kernel.ErrObjectRefUnresolved)
	}, Datasets: cat, Engine: func(instance string) ports.EngineOpener {
		return opensearch.Open(opensearch.Config{URL: endpoint, Namespace: namespace + "/" + instance, PrimaryShards: 1})
	}})
	index, err := common.OpenIndexPlugins(config, common.PluginScope{Repositories: repositories},
		map[string]common.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	pin := reader.DatasetPin{SetID: "sensor-readings", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): receipt.Result.NewCommit}}
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return rd.Require(id, kernel.ErrObjectRefUnresolved)
	}
	if err := index.PrepareDatasetScope(context.Background(), lookup, pin); err != nil {
		t.Fatal(err)
	}
	repo, err = index.DatasetMember(lookup, pin, repo.ID())
	if err != nil {
		t.Fatal(err)
	}
	return memberProjectionFixture{repo: repo, reader: rd, commit: receipt.Result.NewCommit, address: address, access: access,
		index: index, origin: origin, pin: pin, datasets: cat, source: sourcebridge.Source{Lookup: access, Request: objectmodel.RequestContext{
			Identity: kernel.IdentityContext{Principal: "index-worker"}, RequestID: "member-index-observation",
		}}}
}

func memberQuery(value string) retrieval.SearchRequest {
	return retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{
		Schema: "schema/sensor.readings", Aspect: "readings", Path: "value",
	}, Value: value})
}

func assertMemberObservation(t *testing.T, f memberProjectionFixture, result retrieval.SearchResult, revision string) {
	t.Helper()
	if result.Completeness != retrieval.CompletenessComplete || len(result.Hits) != 1 || result.SearchView.Snapshots[f.repo.ID()] != f.commit ||
		result.SearchView.ProjectionRevisions[f.repo.ID()] != revision {
		t.Fatalf("Member query lost its complete fixed observation view: %#v", result)
	}
	hit := result.Hits[0]
	want := map[string]any{"definition": map[string]any{"owner": "fixed"}, "readings": map[string]any{"primary": map[string]any{"value": float64(31)}}}
	if hit.Object.Repository != f.repo.ID() || hit.Object.Commit != f.commit || hit.Object.Address.ObjectID != f.address.ObjectID ||
		kernel.CanonicalDigest(hit.Object.Value) != kernel.CanonicalDigest(want) || len(hit.Version.Observations) != 1 {
		t.Fatalf("Member index read returned its placeholder or another source value: %#v", hit)
	}
	observation := hit.Version.Observations[0]
	if observation.Address != f.address || observation.DeclarationCommit != f.commit || observation.DeclarationDigest == "" ||
		observation.Basis.BindingGeneration != "g1" || observation.Basis.SourceRevision != "r31" {
		t.Fatalf("Member result lost its original successful basis: %#v", hit.Version)
	}
	for _, declaration := range hit.Object.Declarations {
		if declaration.Address == f.address && declaration.SchemaRef == "schema/sensor.readings" && declaration.ValueSource != nil &&
			declaration.ValueSource.Kind == objectmodel.ValueSourceBinding && declaration.DeclarationDigest == observation.DeclarationDigest {
			return
		}
	}
	t.Fatalf("Member result lost the actual adopted Schema and declaration: %#v", hit.Object.Declarations)
}

func TestSelectedLiveAccessMemberProjectionRetainsOriginalObservation(t *testing.T) {
	f := newMemberProjectionFixture(t, "valid")
	published, err := f.index.RefreshState(context.Background(), f.repo, f.commit, f.source)
	if err != nil || published.Revision == "" {
		t.Fatalf("actual selected Member projection did not publish: %#v %v", published, err)
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return f.reader.Require(id, kernel.ErrObjectRefUnresolved)
	}
	request := memberQuery("31")
	result, err := f.index.SearchDataset(context.Background(), lookup, f.pin, request)
	if err != nil {
		t.Fatal(err)
	}
	assertMemberObservation(t, f, result, published.Revision)
	calls := f.origin.calls()
	if len(calls) != 1 || calls[0].Binding.Repository != f.repo.ID() || calls[0].Binding.DeclarationCommit != f.commit ||
		calls[0].Binding.Address != f.address || calls[0].Binding.DeclarationDigest == "" || calls[0].SchemaRef != "schema/sensor.readings" ||
		calls[0].Identity.Principal != "index-worker" || calls[0].Identity.RequestID != "member-index-observation" {
		t.Fatalf("index did not invoke the selected fixed Member source: %#v", calls)
	}
	f.origin.mu.Lock()
	f.origin.value = float64(47)
	f.origin.mu.Unlock()
	retained, err := f.index.SearchDataset(context.Background(), lookup, f.pin, request)
	if err != nil {
		t.Fatal(err)
	}
	assertMemberObservation(t, f, retained, published.Revision)
	current, err := f.index.SearchDataset(context.Background(), lookup, f.pin, memberQuery("47"))
	if err != nil || current.Completeness != retrieval.CompletenessComplete || len(current.Hits) != 0 || len(f.origin.calls()) != len(calls) {
		t.Fatalf("query reobserved the updated source instead of its published basis: %#v calls=%d %v", current, len(f.origin.calls()), err)
	}
	fixed, err := f.repo.ReadAddress(f.address, f.commit)
	if err != nil || fixed.Value != nil ||
		testkit.MustHead(t, f.repo, snapshot.DefaultRef) != f.commit {
		t.Fatalf("index observations changed the fixed Member or Snapshot: %#v %v", fixed, err)
	}
}

func TestFailedLiveAccessMemberCannotPublishStateProjection(t *testing.T) {
	for _, mode := range []string{"source", "basis", "schema", "unselected-source"} {
		t.Run(mode, func(t *testing.T) {
			f := newMemberProjectionFixture(t, mode)
			published, err := f.index.RefreshState(context.Background(), f.repo, f.commit, f.source)
			code := kernel.ErrCapabilityUnsatisfied
			if mode == "source" {
				code = kernel.ErrTemporaryUnavailable
			} else if mode == "schema" {
				code = kernel.ErrSchemaInstanceInvalid
			}
			if kernel.CodeOf(err) != code || published.Revision != "" {
				t.Fatalf("failed Member acquired a successful index revision: %#v %v", published, err)
			}
			if revision, ready := f.index.StateView(f.repo.ID(), f.commit); ready || revision != "" {
				t.Fatalf("failed first observation became a ready state view: %s %v", revision, ready)
			}
			result, err := f.index.SearchDataset(context.Background(), func(id kernel.RepositoryID) (objectmodel.Repository, error) {
				return f.reader.Require(id, kernel.ErrObjectRefUnresolved)
			}, f.pin, memberQuery("31"))
			if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || len(result.Hits) != 0 || result.Completeness == retrieval.CompletenessComplete {
				t.Fatalf("failed Member became a business match or complete zero result: %#v %v", result, err)
			}
			wantCalls := 1
			if mode == "unselected-source" {
				wantCalls = 0
			}
			if len(f.origin.calls()) != wantCalls {
				t.Fatalf("query retried a failed/unselected source: calls=%d want=%d", len(f.origin.calls()), wantCalls)
			}
			if head := testkit.MustHead(t, f.repo, snapshot.DefaultRef); head != f.commit {
				t.Fatalf("failed index preparation changed fixed source: %s", head)
			}
		})
	}
}
