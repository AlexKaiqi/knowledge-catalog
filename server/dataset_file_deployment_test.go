package server

import (
	"bytes"
	"context"
	"encoding/json"
	inv "kc/invocation"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	apphome "kc/home"
	"kc/identity"
	"kc/internal/telemetry"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"kc/snapshot/dataset"
	snapshotservice "kc/snapshot/service"
)

// doc/design/service.md and KS-02: the file gateway must
// borrow declared authorities, keep a fixed basis, and recheck current grants.
// No fixture discovery, implicit resource creation, or new file surface.
func declaredFileGateway(t *testing.T) (apphome.DeploymentConfig, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	fake := testkit.NewLakeFSFake(t)
	cfg := apphome.DeploymentConfig{Version: 1, StateDir: filepath.Join(root, "durable"), CacheDir: filepath.Join(root, "cache"), Auth: "local", BootstrapPrincipal: "agent:operator", Repositories: []apphome.RepositoryBinding{{ID: "kr://kc/system", Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())}}, Registry: apphome.RegistryBinding{ID: "kr://files/catalog", Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())}}
	if err := apphome.InitializeDeployment(cfg, func(dir, principal string) error {
		return inv.WriteAllow(dir, inv.AllowFile{Rules: []inv.AllowRule{
			{ID: "operator", Principal: principal, Actions: []string{"*"}},
			{ID: "file-reader", Principal: "agent:reader", Actions: []string{"dataset.resolve", "file.read"}, Dataset: "protocol"},
		}})
	}); err != nil {
		t.Fatal(err)
	}
	opened, err := apphome.OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := inv.PickDatasets(opened, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.DefineKnowledgeSet("protocol", 1, []dataset.KnowledgeSetSource{{Repository: objectmodel.SystemRepositoryID, Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "deployment.json")
	if err := os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return cfg, configPath
}

func fileGatewayRequest(t *testing.T, handler http.Handler, principal, route string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, route, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Kc-As", principal)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestWorkspaceFileGatewayRestoresDeclaredDeployment(t *testing.T) {
	cfg, configPath := declaredFileGateway(t)
	// The only instance cache may be lost before the gateway opens.
	if err := os.RemoveAll(cfg.CacheDir); err != nil {
		t.Fatal(err)
	}
	handler, err := HTTPHandlerFromConfig(configPath, HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer handler.(interface{ Close() error }).Close()
	var fixedPin dataset.ResolvedKnowledgeSet
	for _, explicit := range []bool{false, true} {
		name := "service Dataset"
		if explicit {
			name = "reject removed Catalog selector"
		}
		t.Run(name, func(t *testing.T) {
			coordinate := map[string]any{"dataset": "protocol", "view": "semantic"}
			if explicit {
				coordinate["catalog"] = cfg.Registry.ID
				response := fileGatewayRequest(t, handler, "agent:reader", "/snapshot/v1/files/mounts:list", coordinate)
				if response.Code != http.StatusBadRequest {
					t.Fatalf("removed selector accepted: %d %s", response.Code, response.Body.String())
				}
				return
			}
			mountResponse := fileGatewayRequest(t, handler, "agent:reader", "/snapshot/v1/files/mounts:list", coordinate)
			if mountResponse.Code != http.StatusOK {
				t.Fatalf("mounts: %d %s", mountResponse.Code, mountResponse.Body.String())
			}
			var mounts workspaceFileMountsResponse
			if err := json.Unmarshal(mountResponse.Body.Bytes(), &mounts); err != nil {
				t.Fatal(err)
			}
			if mounts.Pin.PinID == "" || len(mounts.Mounts) != 1 || mounts.Mounts[0].Repository != objectmodel.SystemRepositoryID {
				t.Fatalf("mounts: %#v", mounts)
			}
			fixedPin = mounts.Pin
			coordinate["pin"], coordinate["mountPath"], coordinate["directory"] = mounts.Pin, mounts.Mounts[0].Path, "_meta"
			listing := fileGatewayRequest(t, handler, "agent:reader", "/snapshot/v1/files/tree:list", coordinate)
			if listing.Code != http.StatusOK {
				t.Fatalf("listing: %d %s", listing.Code, listing.Body.String())
			}
			var page workspaceFileDirectoryResponse
			if err := json.Unmarshal(listing.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if page.Pin.PinID != mounts.Pin.PinID || len(page.Entries) != 1 || page.Entries[0].Name != "repository.yaml" {
				t.Fatalf("directory: %#v", page)
			}
			delete(coordinate, "directory")
			coordinate["file"] = "_meta/repository.yaml"
			read := fileGatewayRequest(t, handler, "agent:reader", "/snapshot/v1/files/file:read", coordinate)
			if read.Code != http.StatusOK {
				t.Fatalf("read: %d %s", read.Code, read.Body.String())
			}
			var value workspaceFileReadResponse
			if err := json.Unmarshal(read.Body.Bytes(), &value); err != nil {
				t.Fatal(err)
			}
			if value.Pin.PinID != mounts.Pin.PinID || !bytes.Contains(value.Content, []byte(objectmodel.SystemRepositoryID)) {
				t.Fatalf("file: %#v", value)
			}
		})
	}
	revoke := httptest.NewRequest(http.MethodDelete, "/admin/v1/grants/file-reader", nil)
	revoke.Header.Set("X-Kc-As", "agent:operator")
	revoked := httptest.NewRecorder()
	handler.ServeHTTP(revoked, revoke)
	if revoked.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", revoked.Code, revoked.Body.String())
	}
	denied := fileGatewayRequest(t, handler, "agent:reader", "/snapshot/v1/files/file:read", map[string]any{
		"dataset": "protocol", "view": "semantic", "pin": fixedPin, "mountPath": "knowledge/system", "file": "_meta/repository.yaml",
	})
	if denied.Code != http.StatusForbidden {
		t.Fatalf("old pin bypassed revoked grant: %d %s", denied.Code, denied.Body.String())
	}
	for _, forbiddenFixture := range []string{"layout.yaml", "repos", "catalogs"} {
		if _, err := os.Stat(filepath.Join(cfg.StateDir, forbiddenFixture)); !os.IsNotExist(err) {
			t.Fatalf("gateway created component fixture %s: %v", forbiddenFixture, err)
		}
	}
}

func TestWorkspaceFileGatewayDeliversSystemKnowledgeTree(t *testing.T) {
	cfg, configPath := declaredFileGateway(t)
	opened, err := apphome.OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := inv.PickDatasets(opened, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cat.DefineKnowledgeSet("protocol", 2, []dataset.KnowledgeSetSource{{Repository: objectmodel.SystemRepositoryID, Selector: snapshot.DefaultRef, Path: dataset.MountPath("knowledge/system")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	handler, err := HTTPHandlerFromConfig(configPath, HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer handler.(interface{ Close() error }).Close()
	coordinate := map[string]any{"dataset": "protocol", "view": "repository"}
	response := fileGatewayRequest(t, handler, "agent:reader", "/snapshot/v1/files/mounts:list", coordinate)
	if response.Code != http.StatusOK {
		t.Fatalf("mounts: %d %s", response.Code, response.Body.String())
	}
	var mounts workspaceFileMountsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &mounts); err != nil {
		t.Fatal(err)
	}
	if len(mounts.Mounts) != 1 || mounts.Mounts[0].Repository != objectmodel.SystemRepositoryID {
		t.Fatalf("system mount: %#v", mounts)
	}
	coordinate["pin"], coordinate["mountPath"], coordinate["directory"] = mounts.Pin, mounts.Mounts[0].Path, "concepts/object"
	listing := fileGatewayRequest(t, handler, "agent:reader", "/snapshot/v1/files/tree:list", coordinate)
	if listing.Code != http.StatusOK {
		t.Fatalf("listing: %d %s", listing.Code, listing.Body.String())
	}
	var page workspaceFileDirectoryResponse
	if err := json.Unmarshal(listing.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Pin.PinID != mounts.Pin.PinID || len(page.Entries) != 1 || page.Entries[0].Name != "definition.md" {
		t.Fatalf("concept directory: %#v", page)
	}
	delete(coordinate, "directory")
	opened, err = apphome.OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	source, err := opened.Store.Require(objectmodel.SystemRepositoryID, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	system, ok := snapshot.TreeReaderOf(source)
	if !ok {
		t.Fatal("system authority lacks literal file reads")
	}
	commit, err := source.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"README.md", "concepts/object/definition.md", "interfaces/writer/definition.md", "mechanisms/governance/definition.md", "mechanisms/hook/definition.md", "capabilities/read-knowledge/entrypoints.yaml", "guides/first-publication/guide.md"} {
		coordinate["file"] = file
		read := fileGatewayRequest(t, handler, "agent:reader", "/snapshot/v1/files/file:read", coordinate)
		if read.Code != http.StatusOK {
			t.Fatalf("read %s: %d %s", file, read.Code, read.Body.String())
		}
		var value workspaceFileReadResponse
		if err := json.Unmarshal(read.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		want, err := system.ReadFile(file, commit)
		if err != nil {
			t.Fatal(err)
		}
		if value.Pin.PinID != mounts.Pin.PinID || !bytes.Equal(value.Content, want) {
			t.Fatalf("%s did not preserve the publication bytes and pin", file)
		}
	}
}

type fileGatewayCloseProbe struct {
	snapshot.Store
	snapshot.TreeReader
	snapshot.DirectoryReader
	closed atomic.Int32
}

func (p *fileGatewayCloseProbe) Close() error {
	p.closed.Add(1)
	return p.Store.(interface{ Close() error }).Close()
}

func TestWorkspaceFileGatewayBorrowsHomeUntilConcurrentReadsFinish(t *testing.T) {
	cfg, _ := declaredFileGateway(t)
	opened, err := apphome.OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	source, _ := opened.Store.Get(objectmodel.SystemRepositoryID)
	probe := &fileGatewayCloseProbe{Store: source, TreeReader: source.(snapshot.TreeReader), DirectoryReader: source.(snapshot.DirectoryReader)}
	opened.Store.Delete(objectmodel.SystemRepositoryID)
	if err := opened.Store.Add(probe); err != nil {
		t.Fatal(err)
	}
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "file-gateway-lifetime-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Shutdown(context.Background())
	facade := &httpFacade{home: cfg.StateDir, readHome: opened, deployment: &cfg, runtime: runtime}
	defer facade.closeReadHome()
	coordinate := snapshotservice.FileCoordinate{Dataset: "protocol", View: "semantic"}
	raw, err := json.Marshal(coordinate)
	if err != nil {
		t.Fatal(err)
	}
	request := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/snapshot/v1/files/mounts:list", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Kc-As", "agent:reader")
		return r
	}
	acquire := func(ctx context.Context, caller identity.HTTPIdentity, operation string) (snapshotservice.Resources, func(), error) {
		resources, release, err := facade.acquireSnapshotResources(ctx, caller, operation)
		if err == nil && resources.Sources != opened.Store {
			t.Error("file request did not borrow the facade-owned source authorities")
		}
		return resources, release, err
	}
	handler := snapshotservice.NewHandler(snapshotservice.Config{
		Identity: facade.snapshotIdentityOptions(), Observe: facade.observeSnapshotOperation, Acquire: acquire,
	})
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, request())
	if first.Code != http.StatusOK || probe.closed.Load() != 0 {
		t.Fatalf("request closed shared Home: status=%d closeCount=%d body=%s", first.Code, probe.closed.Load(), first.Body.String())
	}
	var firstMounts snapshotservice.FileMountsResponse
	if err := json.Unmarshal(first.Body.Bytes(), &firstMounts); err != nil {
		t.Fatal(err)
	}
	if firstMounts.Pin.PinID == "" || len(firstMounts.Mounts) != 1 || firstMounts.Mounts[0].Repository != objectmodel.SystemRepositoryID {
		t.Fatalf("module HTTP request did not open its fixed semantic file view: %#v", firstMounts)
	}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	finished := make(chan *httptest.ResponseRecorder, 2)
	concurrentHandler := snapshotservice.NewHandler(snapshotservice.Config{
		Identity: facade.snapshotIdentityOptions(), Observe: facade.observeSnapshotOperation,
		Acquire: func(ctx context.Context, caller identity.HTTPIdentity, operation string) (snapshotservice.Resources, func(), error) {
			resources, unlock, err := acquire(ctx, caller, operation)
			if err == nil {
				entered <- struct{}{}
				<-release
			}
			return resources, unlock, err
		},
	})
	for range 2 {
		go func() {
			response := httptest.NewRecorder()
			concurrentHandler.ServeHTTP(response, request())
			finished <- response
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("independent file reads were serialized or failed to open")
		}
	}
	closed := make(chan error, 1)
	go func() { closed <- facade.closeReadHome() }()
	select {
	case err := <-closed:
		close(release)
		t.Fatalf("shutdown closed Home during file reads: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	if probe.closed.Load() != 0 {
		close(release)
		t.Fatal("borrowed authority closed before reads completed")
	}
	close(release)
	for range 2 {
		select {
		case response := <-finished:
			if response.Code != http.StatusOK {
				t.Fatalf("file read: %d %s", response.Code, response.Body.String())
			}
		case <-time.After(time.Second):
			t.Fatal("file read did not finish")
		}
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not release Home")
	}
	if probe.closed.Load() != 1 {
		t.Fatalf("authority close count=%d, want one process shutdown", probe.closed.Load())
	}
}
