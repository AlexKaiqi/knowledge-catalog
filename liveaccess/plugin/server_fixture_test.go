package plugin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync/atomic"
	"testing"

	kcclient "kc/client"
	common "kc/discovery/plugin"
	"kc/home"
	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	inv "kc/invocation"
	"kc/kernel"
	"kc/liveaccess"
	accessplugin "kc/liveaccess/plugin"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	objectclient "kc/object/client"
	"kc/server"
	"kc/snapshot"
)

const acceptanceObject objectmodel.ObjectID = "widget/mixed"
const acceptanceR1 kernel.RepositoryID = "kr://liveaccess/r1"
const acceptanceR2 kernel.RepositoryID = "kr://liveaccess/r2"

type acceptanceRuntime struct {
	server   *httptest.Server
	protocol string
	value    atomic.Int64
	calls    atomic.Int64
	attempts atomic.Int64
	mode     atomic.Value
}

func newAcceptanceRuntime(t *testing.T, value int64) *acceptanceRuntime {
	t.Helper()
	r := &acceptanceRuntime{protocol: objectmodel.ProtocolResourceAccessV1}
	r.value.Store(value)
	r.mode.Store("source")
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		r.calls.Add(1)
		var call struct {
			Binding struct {
				Address objectmodel.Address `json:"address"`
			} `json:"binding"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Error(err)
		}
		address := call.Binding.Address
		name := address.AspectName
		if address.MemberKey != "" {
			name = address.MemberKey
		}
		basis := objectmodel.ObservationBasis{BindingGeneration: "runtime-generation", Consistency: objectmodel.ObservationLatestOnly, SourceRevision: fmt.Sprintf("source-%d", r.value.Load()), ObservedAt: "2026-10-14T12:00:00Z"}
		var result any = map[string]any{"value": r.value.Load()}
		if name == "empty" {
			result = nil
		}
		if name == "broken" {
			switch r.mode.Load().(string) {
			case "source":
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "selected source unavailable"}})
				return
			case "basis":
				result, basis = nil, objectmodel.ObservationBasis{}
			case "schema":
				result = map[string]any{"value": "incompatible"}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": result, "basis": basis})
	}))
	t.Cleanup(r.server.Close)
	return r
}

type acceptanceTransport struct {
	target   *url.URL
	attempts *atomic.Int64
}

func (t acceptanceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t.attempts != nil {
		t.attempts.Add(1)
	}
	forward := request.Clone(request.Context())
	address := *request.URL
	address.Scheme, address.Host = t.target.Scheme, t.target.Host
	forward.URL = &address
	return http.DefaultTransport.RoundTrip(forward)
}

type acceptanceFixture struct {
	config  home.DeploymentConfig
	path    string
	sources map[kernel.RepositoryID]snapshot.Store
	commits map[kernel.RepositoryID]kernel.CommitID
	server  *httptest.Server
	handler http.Handler
	options server.HTTPServerOptions
}

func newAcceptanceFixture(t *testing.T, options server.HTTPServerOptions) *acceptanceFixture {
	t.Helper()
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_AS", "")
	t.Setenv("KC_DATASET", "")
	fake := testkit.NewLakeFSFake(t)
	root := t.TempDir()
	f := &acceptanceFixture{sources: map[kernel.RepositoryID]snapshot.Store{}, commits: map[kernel.RepositoryID]kernel.CommitID{}, options: options}
	f.config = home.DeploymentConfig{Version: 1, StateDir: filepath.Join(root, "durable"), CacheDir: filepath.Join(root, "cache"), Auth: "local", BootstrapPrincipal: "operator", Registry: home.RegistryBinding{ID: "kr://liveaccess/registry", Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())}, Repositories: []home.RepositoryBinding{{ID: string(objectmodel.SystemRepositoryID), Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())}}}
	for i, repo := range []kernel.RepositoryID{acceptanceR1, acceptanceR2} {
		dsn := fake.DSN(fake.NewRepo())
		f.config.Repositories = append(f.config.Repositories, home.RepositoryBinding{ID: string(repo), Driver: "lakefs", DSN: dsn})
		store, err := snapshot.OpenLakeFS(repo, dsn, fake.Credential())
		if err != nil {
			t.Fatal(err)
		}
		f.sources[repo] = store
		t.Cleanup(func() {
			if closer, ok := store.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
		})
		f.selectInstance(t, repo, "liveaccess", []string{"one", "two"}[i])
	}
	if err := home.InitializeDeployment(f.config, func(directory, principal string) error {
		return inv.WriteAllow(directory, inv.AllowFile{Rules: []inv.AllowRule{
			{ID: "operator", Principal: principal, Actions: []string{"*"}},
			{ID: "reader", Principal: "reader", Actions: []string{"object.read", "resource.access", "dataset.resolve", "file.read"}},
			{ID: "auditor", Principal: "auditor", Actions: []string{"audit.read"}},
			{ID: "alice", Principal: "alice", Actions: []string{"object.read"}},
			{ID: "bob", Principal: "bob", Actions: []string{"object.read"}},
		}})
	}); err != nil {
		t.Fatal(err)
	}
	opened, err := home.OpenDeployment(f.config)
	if err != nil {
		t.Fatal(err)
	}
	for _, repo := range []kernel.RepositoryID{acceptanceR1, acceptanceR2} {
		if err := opened.AttachRepository(repo); err != nil {
			t.Fatal(err)
		}
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	f.path = filepath.Join(root, "deployment.json")
	f.writeConfig(t)
	f.start(t)
	t.Cleanup(func() { f.stop(t) })
	return f
}

func (f *acceptanceFixture) selectInstance(t *testing.T, repo kernel.RepositoryID, name, instance string) {
	t.Helper()
	raw, err := json.Marshal(common.RepositoryPluginConfig{Version: 1, Plugins: []common.PluginSelection{{Name: name, Instance: instance}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.sources[repo].(interface{ WriteRepositoryMetadata(map[string]string) error }).WriteRepositoryMetadata(map[string]string{common.RepositoryPluginsKey: string(raw)}); err != nil {
		t.Fatal(err)
	}
}
func (f *acceptanceFixture) writeConfig(t *testing.T) {
	t.Helper()
	raw, err := json.Marshal(f.config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}
func (f *acceptanceFixture) start(t *testing.T) {
	t.Helper()
	handler, err := server.HTTPHandlerFromConfig(f.path, f.options)
	if err != nil {
		t.Fatal(err)
	}
	f.handler, f.server = handler, httptest.NewServer(handler)
}
func (f *acceptanceFixture) stop(t *testing.T) {
	t.Helper()
	if f.server == nil {
		return
	}
	f.server.Close()
	if err := f.handler.(interface{ Close() error }).Close(); err != nil {
		t.Error(err)
	}
	f.server = nil
}
func (f *acceptanceFixture) client(t *testing.T, principal string) *kcclient.Client {
	t.Helper()
	config := kcclient.Config{BaseURL: f.server.URL}
	if f.options.Authenticator != nil {
		config.Authenticator = acceptanceClientAuthenticator{}
	}
	client, err := kcclient.New(config)
	if err != nil {
		t.Fatal(err)
	}
	authentication := kcclient.Authentication{}
	if f.options.Authenticator != nil {
		authentication.Authorization = "Bearer " + principal + "-proof"
	}
	if _, err := client.Login(context.Background(), kcclient.LoginRequest{Identity: kernel.IdentityContext{Principal: principal}, Authentication: authentication}); err != nil {
		t.Fatal(err)
	}
	return client
}

type acceptanceClientAuthenticator struct{}

func (acceptanceClientAuthenticator) Login(ctx context.Context, request kcclient.LoginRequest) (kcclient.Session, error) {
	return (kcclient.PassThroughAuthenticator{}).Login(ctx, request)
}
func (acceptanceClientAuthenticator) Logout(context.Context, kcclient.Session) error { return nil }
func (acceptanceClientAuthenticator) AuthenticateRequest(_ context.Context, session kcclient.Session, _ string, request *http.Request) error {
	request.Header.Set("Authorization", session.Authentication.Authorization)
	return nil
}
func (f *acceptanceFixture) publish(t *testing.T, repo kernel.RepositoryID, origin string) {
	t.Helper()
	// Descriptor IDs are unique per request scope, so each Repository names its
	// own; broken has a separate descriptor so tests can move only its origin.
	source, broken := acceptanceDescriptor(repo, "source"), acceptanceDescriptor(repo, "broken")
	operations := []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: acceptanceObject, AspectName: "definition"}, Value: map[string]any{"owner": "fixed"}},
		bindingfixture.PutDescriptor(source, bindingfixture.Descriptor(origin)), bindingfixture.PutDescriptor(broken, bindingfixture.Descriptor(origin))}
	for _, aspect := range []string{"good", "empty", "broken"} {
		schemaRef := "schema/widget." + aspect
		descriptor := source
		if aspect == "broken" {
			descriptor = broken
		}
		operations = append(operations, objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(schemaRef)}, Value: map[string]any{"entity": "Widget", "aspect": aspect, "pattern": "record", "fields": map[string]any{"value": map[string]any{"type": "number"}}}},
			bindingfixture.PutAspect(acceptanceObject, aspect, schemaRef, descriptor))
	}
	operations = append(operations, objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/widget.readings"}, Value: map[string]any{"entity": "Widget", "aspect": "readings", "pattern": "keyed_collection", "fields": map[string]any{"value": map[string]any{"type": "number"}}}},
		bindingfixture.PutMember(acceptanceObject, "readings", "good", "schema/widget.readings", source))
	f.commit(t, repo, operations)
}

func acceptanceDescriptor(repo kernel.RepositoryID, name string) objectmodel.ObjectID {
	return objectmodel.ObjectID("resource/" + path.Base(string(repo)) + "-" + name)
}

func (f *acceptanceFixture) commit(t *testing.T, repo kernel.RepositoryID, operations []objectmodel.Operation) {
	t.Helper()
	head := testkit.MustHead(t, f.sources[repo], snapshot.DefaultRef)
	var receipt struct {
		Result struct {
			NewCommit kernel.CommitID `json:"newCommit"`
		} `json:"result"`
	}
	err := f.client(t, "operator").ObjectService().Commit(context.Background(), string(repo), objectclient.CommitRequest{CommandID: fmt.Sprintf("publish-%s-%s", repo, head), ChangeSet: objectmodel.ChangeSet{TargetRepository: repo, TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head, Operations: operations}}, objectclient.RequestOptions{}, &receipt)
	if err != nil || receipt.Result.NewCommit == "" {
		t.Fatalf("public Writer publication failed: %#v %v", receipt, err)
	}
	f.commits[repo] = receipt.Result.NewCommit
}
func acceptanceReference(t *testing.T, repo kernel.RepositoryID, locator string) string {
	t.Helper()
	reference, err := objectmodel.FormatReference(objectmodel.Reference{Repository: repo, Object: acceptanceObject, Locator: locator})
	if err != nil {
		t.Fatal(err)
	}
	return reference
}

func (f *acceptanceFixture) read(t *testing.T, principal, requestID string, request kcclient.ObjectReadRequest) (*objectmodel.ObservedValue, error) {
	t.Helper()
	var row objectmodel.ObservedValue
	err := f.client(t, principal).Objects().Read(context.Background(), request, kcclient.RequestOptions{RequestID: requestID}, &row)
	if err != nil {
		return nil, err
	}
	selected, err := objectmodel.ParseReference(request.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if row.Reference != request.Reference || row.Repository == "" || request.Repository != "" && string(row.Repository) != request.Repository || row.ObjectID != selected.Object || row.Locator != selected.Locator || row.Commit == "" || request.Commit != "" && string(row.Commit) != request.Commit {
		t.Fatalf("public READ changed explicit object identity or fixed version: request=%#v row=%#v", request, row)
	}
	return &row, nil
}

func acceptanceEntries(t *testing.T, runtimes map[string]*acceptanceRuntime) map[string]liveservice.PluginEntry {
	t.Helper()
	return map[string]liveservice.PluginEntry{"liveaccess": accessplugin.Entry(accessplugin.Options{OpenLookup: func(scope common.PluginScope) (liveaccess.StateLookup, error) {
		runtime := runtimes[scope.Instance]
		if runtime == nil {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "selected runtime instance is unavailable")
		}
		if runtime.protocol != objectmodel.ProtocolResourceAccessV1 {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "selected runtime does not support resource-access/v1")
		}
		target, err := url.Parse(runtime.server.URL)
		if err != nil {
			return nil, err
		}
		return liveaccess.NewHTTPStateLookup(&http.Client{Transport: acceptanceTransport{target: target, attempts: &runtime.attempts}}), nil
	}})}
}
