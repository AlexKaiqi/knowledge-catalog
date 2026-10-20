package cli_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kc/declarativeindex/opensearch"
	indexprojection "kc/declarativeindex/projection"
	index "kc/discovery/projection"
	apphome "kc/home"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	serv "kc/server"
	"kc/snapshot"
)

type serveShutdownFixture struct {
	config      apphome.DeploymentConfig
	configPath  string
	repository  objectmodel.Repository
	commit      kernel.CommitID
	armed       atomic.Bool
	entered     chan struct{}
	canceled    chan struct{}
	heldRequest string
}

func newServeShutdownFixture(t *testing.T) *serveShutdownFixture {
	t.Helper()
	if strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")) == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL must name the local test OpenSearch")
	}
	t.Setenv("KC_EMBEDDING_MODEL", "")
	t.Setenv("KC_RERANK_MODEL", "")
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	root := t.TempDir()
	f := &serveShutdownFixture{entered: make(chan struct{}), canceled: make(chan struct{})}
	var source, heldPath string
	var enteredOnce, canceledOnce sync.Once
	release := make(chan struct{})
	fake := testkit.NewLakeFSFakeWithInterceptor(t, func(_ http.ResponseWriter, r *http.Request) bool {
		if !f.armed.Load() || r.Method != http.MethodGet ||
			!strings.Contains(r.URL.Path, "/repositories/"+source+"/") ||
			!strings.HasSuffix(r.URL.Path, "/objects") ||
			r.URL.Query().Get("path") != heldPath {
			return false
		}
		enteredOnce.Do(func() {
			f.heldRequest = r.URL.RequestURI()
			close(f.entered)
		})
		select {
		case <-r.Context().Done():
			canceledOnce.Do(func() { close(f.canceled) })
		case <-release:
			// Failure cleanup only; passing assertions never release this read.
		}
		return true
	})
	t.Cleanup(func() { close(release) })
	source = fake.NewRepo()
	f.config = apphome.DeploymentConfig{
		Version: 1, StateDir: filepath.Join(root, "durable"), CacheDir: filepath.Join(root, "cache"),
		Auth: "local", BootstrapPrincipal: "agent:shutdown",
		Registry: apphome.RegistryBinding{ID: "kr://shutdown/registry", Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())},
		Repositories: []apphome.RepositoryBinding{
			{ID: "kr://kc/system", Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())},
			{ID: "kr://shutdown/source", Driver: "lakefs", DSN: fake.DSN(source)},
		},
		RepositoryAccess: []apphome.RepositoryAccess{apphome.SystemRepositoryAccess()},
		Stores:           apphome.StoresFile{Index: "none"},
	}
	f.configPath = filepath.Join(root, "deployment.json")
	writeDeployment(t, f.configPath, f.config)
	body(t, deploymentCommand(t, "deployment", "init", "--config", f.configPath))
	f.commit = seedServeShutdownSource(t, f)
	raw, err := snapshot.OpenLakeFS("kr://shutdown/source", fake.DSN(source), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	store := snapshot.NewRegistry()
	if err := store.Add(raw); err != nil {
		t.Fatal(err)
	}
	f.repository, err = reader.NewReader(store).Require(raw.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	heldPath = serveShutdownObjectPath(t, raw, f.commit)
	defaults := apphome.DefaultStores()
	f.config.Stores = apphome.StoresFile{Index: defaults.Index, OpenSearch: defaults.OpenSearch}
	f.config.Stores.OpenSearch.Namespace = "serve-shutdown-" + string(kernel.CanonicalDigest(root))[:20]
	writeDeployment(t, f.configPath, f.config)
	t.Cleanup(func() { cleanupServeShutdownSearch(t, f.config.Stores.OpenSearch) })
	return f
}

func seedServeShutdownSource(t *testing.T, f *serveShutdownFixture) kernel.CommitID {
	t.Helper()
	handler, err := serv.HTTPHandlerFromConfig(f.configPath, serv.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	network := httptest.NewServer(handler)
	defer func() {
		network.Close()
		if closer, ok := handler.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				t.Error(err)
			}
		}
	}()
	call := func(args ...string) kcRunResult {
		return kcRemote(t, network.URL, f.config.BootstrapPrincipal, args...)
	}
	body(t, call("attach", "--repo", "kr://shutdown/source"))
	published := asMap(t, body(t, call("writer", "commit", "--command-id", "shutdown-seed",
		"--repo", "kr://shutdown/source", "--dir", writeProviderDrafts(t))))
	return kernel.CommitID(publishedCommit(t, published))
}

func serveShutdownObjectPath(t *testing.T, raw snapshot.Store, commit kernel.CommitID) string {
	t.Helper()
	tree := raw.(snapshot.TreeReader)
	files, err := tree.ListFiles(commit)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		content, err := tree.ReadFile(name, commit)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), "切换支付流量前先检查冻结窗口") {
			return name
		}
	}
	t.Fatal("published fixture did not retain the expected object body")
	return ""
}

func (f *serveShutdownFixture) describe(t *testing.T) index.IndexDescriptor {
	t.Helper()
	inspection := indexprojection.NewIndexEngine(t.TempDir(), opensearch.Open(f.config.Stores.OpenSearch))
	defer func() {
		if err := inspection.Close(); err != nil {
			t.Error(err)
		}
	}()
	desc, err := inspection.Describe(f.repository)
	if err != nil {
		t.Fatal(err)
	}
	return desc
}

func (f *serveShutdownFixture) waitReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var desc index.IndexDescriptor
	for time.Now().Before(deadline) {
		desc = f.describe(t)
		if desc.BasisCommit == f.commit && desc.State == index.ProjectionStateReady && desc.ObjectCount > 0 {
			t.Logf("restarted source recovered automatically at %s without explicit sync", f.commit)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("restarted source did not recover its projection without explicit sync: %#v", desc)
}
