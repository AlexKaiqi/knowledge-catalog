package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

func TestSystemSchemaDiscoveryIsBoundedAndWorkspaceIndependent(t *testing.T) {
	home := t.TempDir()
	mustWorkspaceFSRun(t, home, "init", "--catalog", "kr://acme/catalog")
	server := httptest.NewServer(HTTPHandler(home))
	defer server.Close()

	request := func(payload map[string]any) (int, map[string]any, string) {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		httpRequest, err := http.NewRequest(http.MethodPost, server.URL+"/knowledge/v1/schemas:list", bytes.NewReader(raw))
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
		var body map[string]any
		_ = json.Unmarshal(responseBody, &body)
		return response.StatusCode, body, string(responseBody)
	}

	page := func(continuation string) map[string]any {
		t.Helper()
		payload := map[string]any{"repository": "kr://kc/system", "limit": 2}
		if continuation != "" {
			payload["continuation"] = continuation
		}
		status, body, raw := request(payload)
		if status != http.StatusOK {
			t.Fatalf("schema page returned %d: %s", status, raw)
		}
		return body
	}

	wantTotal := len(knowledge.SystemSchemaOperations())
	first := page("")
	if first["repository"] != "kr://kc/system" || first["exhausted"] != false {
		t.Fatalf("unexpected first page: %#v", first)
	}
	if schemas, ok := first["schemas"].([]any); !ok || len(schemas) != 2 {
		t.Fatalf("unexpected first schema page: %#v", first["schemas"])
	}
	second := page(first["continuation"].(string))
	if second["exhausted"] != true {
		t.Fatalf("second page must exhaust system schemas: %#v", second)
	}
	coverage := second["coverage"].(map[string]any)
	if coverage["total"] != float64(wantTotal) {
		t.Fatalf("unexpected coverage: %#v want %d", coverage, wantTotal)
	}

	status, zero, raw := request(map[string]any{"repository": "kr://kc/system", "limit": 0})
	if status != http.StatusOK || zero["exhausted"] != true || len(zero["schemas"].([]any)) != wantTotal {
		t.Fatalf("schema page limit 0 must mean the default page: status=%d payload=%#v raw=%s", status, zero, raw)
	}
	status, oversized, raw := request(map[string]any{"repository": "kr://kc/system", "limit": 201})
	errObj, _ := oversized["error"].(map[string]any)
	if status != http.StatusBadRequest || errObj["code"] != string(kernel.ErrUsageInvalid) {
		t.Fatalf("schema page limit 201 status=%d payload=%#v raw=%s", status, oversized, raw)
	}
}
