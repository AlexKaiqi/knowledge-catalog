package home

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kc/declarativeindex/opensearch"
	retrieval "kc/discovery"
	consumerplugin "kc/discovery/plugin"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

type connectedPluginFixture struct {
	config        DeploymentConfig
	id            kernel.RepositoryID
	basis         kernel.CommitID
	canonical     objectmodel.ObjectValue
	credential    atomic.Value
	metadataFault atomic.Int64
	metadataReads atomic.Int64
	requests      atomic.Int64
}

func newConnectedPluginFixture(t *testing.T) *connectedPluginFixture {
	t.Helper()
	f := &connectedPluginFixture{id: "kr://connected/index-runtime"}
	f.credential.Store("access:original-secret")
	provider := testkit.NewLakeFSFakeWithInterceptor(t, func(w http.ResponseWriter, r *http.Request) bool {
		f.requests.Add(1)
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			return false
		}
		user, secret, ok := r.BasicAuth()
		if !ok || user+":"+secret != f.credential.Load().(string) {
			w.WriteHeader(http.StatusUnauthorized)
			return true
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/metadata") {
			f.metadataReads.Add(1)
			if status := f.metadataFault.Load(); status != 0 {
				w.WriteHeader(int(status))
				return true
			}
		}
		return false
	})
	dsn := provider.DSN(provider.NewRepo())
	source, err := snapshot.OpenLakeFS(f.id, dsn, f.credential.Load().(string))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeManagedSource(source) })
	writeConnectedPluginConfig(t, source, []consumerplugin.PluginSelection{{Name: "declarative-index", Instance: "main"}})
	repo := testkit.OpenRepository(t, source)
	root := testkit.MustHead(t, source, snapshot.DefaultRef)
	f.basis, err = repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: f.id, TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{
				"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/current"}, SchemaRef: "schema/article", Value: map[string]any{
				"body": "runbook from the connected authority", "detail": "canonical content absent from indexed fields",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.canonical, err = repo.Read("article/current", f.basis)
	if err != nil {
		t.Fatal(err)
	}
	f.config = deploymentFixture(t)
	f.config.Stores.Index = "opensearch"
	namespace := string(kernel.CanonicalDigest(t.TempDir()))
	f.config.Stores.OpenSearch = opensearch.Config{
		URL: connectedPluginSearchEndpoint(t, namespace), Namespace: namespace, PrimaryShards: 1,
	}
	f.config.Connections = &ConnectionPolicy{AllowedOrigins: []string{provider.Origin()}, CreatorActions: []string{
		"repository.connections.manage", "repository.metadata.read", "object.read", "object.search",
	}}
	if err := InitializeDeployment(f.config, func(dir, _ string) error {
		return os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600)
	}); err != nil {
		t.Fatal(err)
	}
	ws, err := OpenDeployment(f.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	if _, err := ws.ConnectRepository(ConnectionRequest{
		Repository: string(f.id), Driver: "lakefs", URL: dsn,
		Principal: "alice", Credential: f.credential.Load().(string),
	}, func(RepositoryInitialGrant) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := ws.AttachRepository(f.id); err != nil {
		t.Fatal(err)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *connectedPluginFixture) openCold(t *testing.T) *Home {
	t.Helper()
	before := f.requests.Load()
	ws, err := OpenDeployment(f.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	if f.requests.Load() != before {
		t.Fatal("cold connection recovery contacted its provider")
	}
	return ws
}

func writeConnectedPluginConfig(t *testing.T, source snapshot.Store, selections []consumerplugin.PluginSelection) {
	t.Helper()
	raw, err := json.Marshal(consumerplugin.RepositoryPluginConfig{Version: 1, Plugins: selections})
	if err != nil {
		t.Fatal(err)
	}
	if err := source.(metadataWrapperWriter).WriteRepositoryMetadata(map[string]string{consumerplugin.RepositoryPluginsKey: string(raw)}); err != nil {
		t.Fatal(err)
	}
}

func (f *connectedPluginFixture) frozen(t *testing.T, ws *Home) dataset.Dataset {
	t.Helper()
	def, err := ws.Datasets.PrepareDataset("connected-articles", 1, []dataset.DatasetSource{{
		Repository: f.id, Selector: snapshot.DefaultRef, BaseRev: string(f.basis), Path: dataset.MountPath("articles"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	return def
}

func (f *connectedPluginFixture) prepareAndRead(t *testing.T, ws *Home) {
	t.Helper()
	if !ws.DatasetPreparation.Enabled || ws.DatasetPreparation.Provider != ws.Plugins || ws.Index != ws.Plugins {
		t.Fatal("runtime configuration did not reach query and Dataset preparation together")
	}
	def := f.frozen(t, ws)
	if err := ws.DatasetPreparation.Execute(context.Background(), def); err != nil {
		t.Fatal(err)
	}
	f.assertCanonical(t, ws)
}

func (f *connectedPluginFixture) assertCanonical(t *testing.T, ws *Home) {
	t.Helper()
	repo, err := ws.Reader.Require(f.id, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ws.Index.SearchAtContext(context.Background(), repo, f.basis, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || result.Completeness != retrieval.CompletenessComplete || len(result.Hits) != 1 || result.SearchView.Snapshots[f.id] != f.basis {
		t.Fatalf("verified connected plugin did not serve its fixed complete result: result=%#v error=%v", result, err)
	}
	hit := result.Hits[0]
	if !reflect.DeepEqual(hit.Object, f.canonical) || !reflect.DeepEqual(hit.Version, retrieval.VersionOf(f.canonical)) || len(hit.Evidence) == 0 {
		t.Fatalf("plugin returned indexed fields instead of fixed canonical object and version: hit=%#v want=%#v", hit, f.canonical)
	}
	if head := testkit.MustHead(t, repo, snapshot.DefaultRef); head != f.basis {
		t.Fatalf("plugin assembly or preparation changed the source HEAD: %s", head)
	}
}

// Forward every engine operation to the already-running real OpenSearch. Record
// only resources actually created by this test so cleanup never scans or deletes
// another test's indices, including the shared control index.
func connectedPluginSearchEndpoint(t *testing.T, namespace string) string {
	t.Helper()
	endpoint := strings.TrimRight(os.Getenv("KC_TEST_OPENSEARCH_URL"), "/")
	if endpoint == "" {
		t.Fatal("connected plugin runtime tests require the existing real OpenSearch endpoint")
	}
	target, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	indices, documents := map[string]bool{}, map[string]bool{}
	controlIndex := "kc-projection-control-v1-" + string(kernel.CanonicalDigest(namespace)[:24])
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(response *http.Response) error {
		request := response.Request
		if request.Method == http.MethodPut && response.StatusCode < 300 {
			path := strings.Trim(request.URL.Path, "/")
			if strings.HasPrefix(path, "kc-projection-control-v1") && strings.SplitN(path, "/", 2)[0] != controlIndex {
				t.Errorf("main plugin discarded the host's resource namespace: path=%s want control=%s", path, controlIndex)
			}
			mu.Lock()
			if !strings.Contains(path, "/") && path != "kc-projection-control-v1" {
				indices[path] = true
			} else if strings.Contains(path, "/_doc/") {
				documents[path] = true
			}
			mu.Unlock()
		}
		return nil
	}
	forwarder := httptest.NewServer(proxy)
	t.Cleanup(forwarder.Close)
	t.Cleanup(func() {
		client := &http.Client{Timeout: 10 * time.Second}
		for _, group := range []map[string]bool{documents, indices} {
			for path := range group {
				request, err := http.NewRequest(http.MethodDelete, endpoint+"/"+path, nil)
				if err != nil {
					t.Error(err)
					continue
				}
				response, err := client.Do(request)
				if err != nil {
					t.Errorf("remove connected test resource %s: %v", path, err)
					continue
				}
				_ = response.Body.Close()
				if response.StatusCode >= 300 && response.StatusCode != http.StatusNotFound {
					t.Errorf("remove connected test resource %s: HTTP %d", path, response.StatusCode)
				}
			}
		}
	})
	return forwarder.URL
}
