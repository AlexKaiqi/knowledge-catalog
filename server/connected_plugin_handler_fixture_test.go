package server

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	retrieval "kc/discovery"
	consumerplugin "kc/discovery/plugin"
	index "kc/discovery/projection"
	apphome "kc/home"
	"kc/internal/testkit"
	inv "kc/invocation"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

type connectedHandlerFixture struct {
	configPath    string
	principal     string
	source        snapshot.Store
	canonical     objectmodel.ObjectValue
	requests      atomic.Int64
	metadataReads atomic.Int64
	engineWrites  *atomic.Int64
}

func newConnectedHandlerFixture(t *testing.T) *connectedHandlerFixture {
	t.Helper()
	f := &connectedHandlerFixture{}
	credential := "access:handler-connection-secret"
	provider := testkit.NewLakeFSFakeWithInterceptor(t, func(w http.ResponseWriter, r *http.Request) bool {
		f.requests.Add(1)
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			return false
		}
		user, secret, ok := r.BasicAuth()
		if !ok || user+":"+secret != credential {
			w.WriteHeader(http.StatusUnauthorized)
			return true
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/metadata") {
			f.metadataReads.Add(1)
		}
		return false
	})
	id := kernel.RepositoryID("kr://handler/connected-object")
	dsn := provider.DSN(provider.NewRepo())
	var err error
	f.source, err = snapshot.OpenLakeFS(id, dsn, credential)
	if err != nil {
		t.Fatal(err)
	}
	f.writeConfig(t, []consumerplugin.PluginSelection{{Name: "declarative-index", Instance: "main"}})
	repo := testkit.OpenRepository(t, f.source)
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	at, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: id, TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{
				"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/current"}, SchemaRef: "schema/article", Value: map[string]any{
				"body": "runbook from connected authority", "detail": "canonical field outside the index declaration",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.canonical, err = repo.Read("article/current", at)
	if err != nil {
		t.Fatal(err)
	}
	cfg, path := declaredFileGateway(t)
	f.configPath, f.principal = path, cfg.BootstrapPrincipal
	cfg.Connections = &apphome.ConnectionPolicy{AllowedOrigins: []string{provider.Origin()}, CreatorActions: []string{
		"repository.connections.manage", "repository.metadata.read", "object.read", "object.search",
	}}
	cfg.Stores.Index = "opensearch"
	namespace := string(kernel.CanonicalDigest(t.TempDir()))
	cfg.Stores.OpenSearch.URL, f.engineWrites = connectedHandlerEngineEndpoint(t, namespace)
	cfg.Stores.OpenSearch.Namespace, cfg.Stores.OpenSearch.PrimaryShards = namespace, 1
	// This is an explicit initialization of the newly declared connection
	// ledger on the same actual deployment, preserving its existing policies.
	if err := apphome.InitializeDeployment(cfg, func(string, string) error {
		t.Fatal("existing deployment unexpectedly reissued bootstrap policy")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ws, err := apphome.OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	if _, err := ws.ConnectRepository(apphome.ConnectionRequest{
		Repository: string(id), Driver: "lakefs", URL: dsn, Principal: f.principal, Credential: credential,
	}, func(grant apphome.RepositoryInitialGrant) error {
		return inv.EnsureManagedRepositoryGrant(cfg.StateDir, grant)
	}); err != nil {
		t.Fatal(err)
	}
	if err := ws.AttachRepository(id); err != nil {
		t.Fatal(err)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *connectedHandlerFixture) writeConfig(t *testing.T, selections []consumerplugin.PluginSelection) {
	t.Helper()
	raw, err := json.Marshal(consumerplugin.RepositoryPluginConfig{Version: 1, Plugins: selections})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.source.(interface{ WriteRepositoryMetadata(map[string]string) error }).WriteRepositoryMetadata(map[string]string{
		consumerplugin.RepositoryPluginsKey: string(raw),
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *connectedHandlerFixture) handler(t *testing.T) http.Handler {
	t.Helper()
	handler, err := HTTPHandlerFromConfig(f.configPath, HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handler.(interface{ Close() error }).Close() })
	return handler
}

func (f *connectedHandlerFixture) query(t *testing.T, handler http.Handler) retrieval.SearchResult {
	t.Helper()
	response := fileGatewayRequest(t, handler, f.principal, "/object/v1/search", map[string]any{
		"repository": f.canonical.Repository, "commit": f.canonical.Commit, "query": "runbook", "limit": 10,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("activated handler query failed: status=%d body=%s", response.Code, response.Body.String())
	}
	var result retrieval.SearchResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Completeness != retrieval.CompletenessComplete || len(result.Hits) != 1 || result.SearchView.Snapshots[f.canonical.Repository] != f.canonical.Commit {
		t.Fatalf("activated handler did not keep its fixed complete search basis: %#v", result)
	}
	hit := result.Hits[0]
	if hit.Object.Repository != f.canonical.Repository || hit.Object.Commit != f.canonical.Commit || hit.Object.Address != f.canonical.Address ||
		!reflect.DeepEqual(hit.Object.Value, f.canonical.Value) || !reflect.DeepEqual(hit.Version, retrieval.VersionOf(f.canonical)) || len(hit.Evidence) == 0 {
		t.Fatalf("activated handler lost fixed canonical contents or version: hit=%#v want=%#v", hit, f.canonical)
	}
	return result
}

func (f *connectedHandlerFixture) waitForStartedProjection(t *testing.T, handler http.Handler) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		response := fileGatewayRequest(t, handler, f.principal, "/operations/v1/projections:describe", map[string]any{
			"repository": f.canonical.Repository, "commit": f.canonical.Commit,
		})
		last = response.Body.String()
		var descriptor index.IndexDescriptor
		if response.Code == http.StatusOK && json.Unmarshal(response.Body.Bytes(), &descriptor) == nil &&
			descriptor.State == index.ProjectionStateReady && descriptor.BasisCommit == f.canonical.Commit {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("started actual worker never prepared its fixed published basis: %s", last)
}
