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
	"kc/internal/testkit"
	"kc/kernel"
	sourcebridge "kc/liveaccess/discovery"
	accessplugin "kc/liveaccess/plugin"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
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
	repo    objectmodel.Repository
	reader  *reader.Reader
	commit  kernel.CommitID
	address objectmodel.Address
	access  *liveservice.Plugins
	index   *common.IndexPlugins
	source  sourcebridge.Source
	origin  *memberOrigin
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
	receipt, err := w.Commit("publish-live-member", objectmodel.ChangeSet{TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root, Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/sensor.readings"}, Value: map[string]any{
				"entity": "Sensor", "aspect": "readings", "pattern": "keyed_collection", "origin": runtime.URL,
				"fields": map[string]any{"value": map[string]any{"type": "number", "required": true, "access": []any{"filter", "sort"}}},
			}},
			{Op: objectmodel.OpPut, Address: address, SchemaRef: "schema/sensor.readings", Value: map[string]any{"value": float64(7)}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: address.ObjectID, AspectName: "definition"}, Value: map[string]any{"owner": "fixed"}},
		}})
	if err != nil {
		t.Fatal(err)
	}
	rd := reader.NewReader(registry)
	repo, err := rd.Require(raw.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	config := common.RepositoryPluginConfig{Version: 1, Plugins: []common.PluginSelection{{Name: "liveaccess", Instance: "main"}, {Name: "declarative-index", Instance: "main"}}}
	if mode == "unselected-source" {
		config.Plugins = config.Plugins[1:]
	}
	configs := common.RepositoryPluginConfigs{Repositories: map[kernel.RepositoryID]common.RepositoryPluginConfig{repo.ID(): config}}
	known := []string{"liveaccess", "declarative-index"}
	access, err := liveservice.OpenPlugins(common.SelectRepositoryPlugins(configs, []string{"liveaccess"}, known),
		map[string]liveservice.PluginEntry{"liveaccess": accessplugin.Entry(accessplugin.Options{})})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = access.Close() })
	entry := indexplugin.Entry(indexplugin.Options{Directory: t.TempDir(), Lookup: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return rd.Require(id, kernel.ErrObjectRefUnresolved)
	}, Datasets: testkit.OpenDatasets(t, registry), Engine: func(instance string) ports.EngineOpener {
		return opensearch.Open(opensearch.Config{URL: endpoint, Namespace: namespace + "/" + instance, PrimaryShards: 1})
	}})
	index, err := common.OpenIndexPlugins(common.SelectRepositoryPlugins(configs, []string{"declarative-index"}, known),
		map[string]common.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	return memberProjectionFixture{repo: repo, reader: rd, commit: receipt.Result.NewCommit, address: address, access: access,
		index: index, origin: origin, source: sourcebridge.Source{Lookup: access, Request: objectmodel.RequestContext{
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
	executor := discoveryservice.SearchExecutor{Repositories: f.reader, Projection: f.index}
	request := discoveryservice.SearchRequest{Repository: f.repo.ID(), Commit: f.commit, Query: memberQuery("31")}
	result, err := executor.Execute(context.Background(), request)
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
	retained, err := f.index.SearchStateAtRevisionContext(context.Background(), f.repo, f.commit, published.Revision, request.Query)
	if err != nil {
		t.Fatal(err)
	}
	assertMemberObservation(t, f, retained, published.Revision)
	current, err := executor.Execute(context.Background(), discoveryservice.SearchRequest{Repository: f.repo.ID(), Commit: f.commit, Query: memberQuery("47")})
	if err != nil || current.Completeness != retrieval.CompletenessComplete || len(current.Hits) != 0 || len(f.origin.calls()) != len(calls) {
		t.Fatalf("query reobserved the updated source instead of its published basis: %#v calls=%d %v", current, len(f.origin.calls()), err)
	}
	fixed, err := f.repo.ReadAddress(f.address, f.commit)
	if err != nil || kernel.CanonicalDigest(fixed.Value) != kernel.CanonicalDigest(map[string]any{"value": float64(7)}) ||
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
			result, err := (discoveryservice.SearchExecutor{Repositories: f.reader, Projection: f.index}).Execute(context.Background(),
				discoveryservice.SearchRequest{Repository: f.repo.ID(), Commit: f.commit, Query: memberQuery("31")})
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
