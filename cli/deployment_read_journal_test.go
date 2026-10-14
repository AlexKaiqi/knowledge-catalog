package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serv "kc/server"
)

func TestDeploymentRepositoryReadJournalFailureSuppressesDelivery(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := serv.HTTPHandlerFromConfig(path, serv.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	read := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/knowledge/v1/objects:read", strings.NewReader(`{"reference":"urn:kc:object:kr%3A%2F%2Fkc%2Fsystem:schema%2Fmeta%2Fschema-definition%2Fv1"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Kc-As", "agent:operator")
		request.Header.Set("X-Kc-Request-Id", "mandatory-reader-journal")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := read(); response.Code != http.StatusOK {
		t.Fatalf("unblocked fixed read failed: %d %s", response.Code, response.Body.String())
	}
	journalPath := filepath.Join(cfg.StateDir, "system.jsonl")
	if err := os.Remove(journalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(journalPath, 0700); err != nil {
		t.Fatal(err)
	}
	response := read()
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code == http.StatusOK || len(payload["error"]) == 0 || len(payload["value"]) != 0 {
		t.Fatalf("unrecorded Reader result was delivered: %d %s", response.Code, response.Body.String())
	}
}
