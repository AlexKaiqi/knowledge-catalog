package cli_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"kc/cli"
	"kc/internal/journal"
)

func TestDeploymentConcurrentTypedReadsKeepRequestStateIsolated(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	catalogPath := "/catalog/v1/catalogs/" + url.PathEscape(cfg.Catalogs[0].ID)
	call := func(method, path, body, principal, requestID string) error {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Kc-As", principal)
		request.Header.Set("X-Kc-Request-Id", requestID)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			return fmt.Errorf("%s %s: %d %s", method, path, response.Code, response.Body.String())
		}
		return nil
	}
	if err := call(http.MethodPost, catalogPath+"/workspaces", `{"workspace":"system","revision":1,"sources":[{"repository":"kr://kc/system","selector":"refs/heads/main"}]}`, "agent:operator", "setup"); err != nil {
		t.Fatal(err)
	}
	requests := []struct{ method, path, body string }{
		{http.MethodGet, catalogPath, ""},
		{http.MethodPost, catalogPath + "/workspaces/system/resolve", `{}`},
		{http.MethodPost, catalogPath + "/workspaces/system/check", `{}`},
		{http.MethodPost, "/knowledge/v1/objects:read", `{"repository":"kr://kc/system","object":"schema/meta/schema-definition/v1"}`},
		{http.MethodPost, "/workspace-files/v1/mounts:list", `{"workspace":"system","view":"semantic"}`},
	}
	start := make(chan struct{})
	errors := make(chan error, len(requests)*6)
	var waiting sync.WaitGroup
	for worker := 0; worker < 6; worker++ {
		for index, item := range requests {
			waiting.Add(1)
			go func() {
				defer waiting.Done()
				<-start
				principal := "agent:operator"
				if index == 3 {
					principal = fmt.Sprintf("agent:reader-%d", worker)
				}
				if err := call(item.method, item.path, item.body, principal, fmt.Sprintf("reader-%d-%d", worker, index)); err != nil {
					errors <- err
				}
			}()
		}
	}
	close(start)
	waiting.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	events, err := journal.Read(filepath.Join(cfg.StateDir, "system.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for worker := 0; worker < 6; worker++ {
		requestID := fmt.Sprintf("reader-%d-3", worker)
		principal := fmt.Sprintf("agent:reader-%d", worker)
		found := false
		for _, event := range events {
			if event.Face != "reader" || event.RequestID != requestID {
				continue
			}
			found = true
			if event.Principal != principal || event.As != principal || event.OnBehalfOf != "" {
				t.Errorf("request %s inherited another identity: %#v", requestID, event)
			}
		}
		if !found {
			t.Errorf("request %s has no Reader audit event", requestID)
		}
	}
}
