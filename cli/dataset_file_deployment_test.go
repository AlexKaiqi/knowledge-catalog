package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"kc/catalog"
	apphome "kc/home"
	"kc/internal/telemetry"
	"kc/internal/testkit"
	"kc/knowledge"
	"kc/snapshot"
)

// SERVICE_ARCHITECTURE §§3.4, 3.6, 11.1 and KS-02: the file gateway must
// borrow declared authorities, keep a fixed basis, and recheck current grants.
// No fixture discovery, implicit resource creation, or new file surface.
func declaredFileGateway(t *testing.T) (apphome.DeploymentConfig, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	fake := testkit.NewLakeFSFake(t)
	cfg := apphome.DeploymentConfig{Version: 1, StateDir: filepath.Join(root, "durable"), CacheDir: filepath.Join(root, "cache"), Auth: "local", BootstrapPrincipal: "agent:operator", Catalogs: []apphome.CatalogBinding{{ID: "kr://files/catalog", Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())}}}
	if err := apphome.InitializeDeployment(cfg, func(dir, principal string) error {
		return WriteAllow(dir, AllowFile{Rules: []AllowRule{
			{ID: "operator", Principal: principal, Actions: []string{"*"}},
			{ID: "file-reader", Principal: "agent:reader", Actions: []string{"dataset.resolve", "file.read"}, Catalog: cfg.Catalogs[0].ID, Dataset: "protocol"},
		}})
	}); err != nil {
		t.Fatal(err)
	}
	opened, err := apphome.OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cat, _, err := opened.UseCatalog(cfg.Catalogs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.DefineKnowledgeSet("protocol", 1, []catalog.KnowledgeSetSource{{Repository: knowledge.SystemRepositoryID, Selector: snapshot.DefaultRef}}); err != nil {
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
	var fixedPin catalog.ResolvedKnowledgeSet
	for _, explicit := range []bool{false, true} {
		name := "default Catalog"
		if explicit {
			name = "explicit Catalog"
		}
		t.Run(name, func(t *testing.T) {
			coordinate := map[string]any{"dataset": "protocol", "view": "semantic"}
			if explicit {
				coordinate["catalog"] = cfg.Catalogs[0].ID
			}
			mountResponse := fileGatewayRequest(t, handler, "agent:reader", "/dataset-files/v1/mounts:list", coordinate)
			if mountResponse.Code != http.StatusOK {
				t.Fatalf("mounts: %d %s", mountResponse.Code, mountResponse.Body.String())
			}
			var mounts workspaceFileMountsResponse
			if err := json.Unmarshal(mountResponse.Body.Bytes(), &mounts); err != nil {
				t.Fatal(err)
			}
			if mounts.Pin.PinID == "" || len(mounts.Mounts) != 1 || mounts.Mounts[0].Repository != knowledge.SystemRepositoryID {
				t.Fatalf("mounts: %#v", mounts)
			}
			fixedPin = mounts.Pin
			coordinate["pin"], coordinate["mountPath"], coordinate["directory"] = mounts.Pin, mounts.Mounts[0].Path, "_meta"
			listing := fileGatewayRequest(t, handler, "agent:reader", "/dataset-files/v1/tree:list", coordinate)
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
			read := fileGatewayRequest(t, handler, "agent:reader", "/dataset-files/v1/file:read", coordinate)
			if read.Code != http.StatusOK {
				t.Fatalf("read: %d %s", read.Code, read.Body.String())
			}
			var value workspaceFileReadResponse
			if err := json.Unmarshal(read.Body.Bytes(), &value); err != nil {
				t.Fatal(err)
			}
			if value.Pin.PinID != mounts.Pin.PinID || !bytes.Contains(value.Content, []byte(knowledge.SystemRepositoryID)) {
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
	denied := fileGatewayRequest(t, handler, "agent:reader", "/dataset-files/v1/file:read", map[string]any{
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

type fileGatewayCloseProbe struct {
	*knowledge.SystemRepository
	closed atomic.Int32
}

func (p *fileGatewayCloseProbe) Close() error { p.closed.Add(1); return nil }

func TestWorkspaceFileGatewayBorrowsHomeUntilConcurrentReadsFinish(t *testing.T) {
	cfg, _ := declaredFileGateway(t)
	opened, err := apphome.OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	probe := &fileGatewayCloseProbe{SystemRepository: knowledge.NewSystemRepository()}
	opened.Store = snapshot.NewRegistry()
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
	coordinate := workspaceFileCoordinate{Dataset: "protocol", View: "semantic"}
	request := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/dataset-files/v1/mounts:list", nil)
		r.Header.Set("X-Kc-As", "agent:reader")
		return r
	}
	first := httptest.NewRecorder()
	facade.withWorkspaceFiles(first, request(), "file-mounts", coordinate, false, func(view *workspaceFileView) (any, error) {
		if view.opened != opened {
			t.Error("file request did not borrow the facade Home")
		}
		return workspaceFileMountsResponse{Pin: view.pin, Mounts: view.mounts}, nil
	})
	if first.Code != http.StatusOK || probe.closed.Load() != 0 {
		t.Fatalf("request closed shared Home: status=%d closeCount=%d body=%s", first.Code, probe.closed.Load(), first.Body.String())
	}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	finished := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() {
			response := httptest.NewRecorder()
			facade.withWorkspaceFiles(response, request(), "file-mounts", coordinate, false, func(view *workspaceFileView) (any, error) {
				entered <- struct{}{}
				<-release
				return workspaceFileMountsResponse{Pin: view.pin, Mounts: view.mounts}, nil
			})
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
