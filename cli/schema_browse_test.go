package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSystemSchemaDiscoveryIsBoundedAndWorkspaceIndependent(t *testing.T) {
	home := t.TempDir()
	mustWorkspaceFSRun(t, home, "init", "--catalog", "kr://acme/catalog")
	server := httptest.NewServer(HTTPHandler(home))
	defer server.Close()

	request := func(continuation string) map[string]any {
		payload := map[string]any{"repository": "kr://kc/system", "limit": 2}
		if continuation != "" {
			payload["continuation"] = continuation
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		httpRequest, err := http.NewRequest(http.MethodPost, server.URL+"/knowledge/v1/schemas:page", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		httpRequest.Header.Set("Content-Type", "application/json")
		httpRequest.Header.Set("X-Kc-As", "user:reader")
		response, err := server.Client().Do(httpRequest)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		responseBody, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("schema page returned %d: %s", response.StatusCode, responseBody)
		}
		var body map[string]any
		if err := json.Unmarshal(responseBody, &body); err != nil {
			t.Fatal(err)
		}
		return body
	}

	first := request("")
	if first["repository"] != "kr://kc/system" || first["exhausted"] != false {
		t.Fatalf("unexpected first page: %#v", first)
	}
	if schemas, ok := first["schemas"].([]any); !ok || len(schemas) != 2 {
		t.Fatalf("unexpected first schema page: %#v", first["schemas"])
	}
	second := request(first["continuation"].(string))
	if second["exhausted"] != true {
		t.Fatalf("second page must exhaust system schemas: %#v", second)
	}
	coverage := second["coverage"].(map[string]any)
	if coverage["total"] != float64(3) {
		t.Fatalf("unexpected coverage: %#v", coverage)
	}
}
