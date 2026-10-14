package cli

import (
	"bytes"
	"encoding/json"
	"io"
	serv "kc/server"
	"kc/systemrepo"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/kernel"
)

func TestSystemSchemaDiscoveryIsBoundedAndWorkspaceIndependent(t *testing.T) {
	home := t.TempDir()
	mustDatasetFSRun(t, home, "init")
	server := httptest.NewServer(serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{}))
	defer server.Close()

	request := func(payload map[string]any) (int, map[string]any, string) {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		httpRequest, err := http.NewRequest(http.MethodPost, server.URL+"/object/v1/schemas:list", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		httpRequest.Header.Set("Content-Type", "application/json")
		httpRequest.Header.Set("X-Kc-As", "reader")
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

	wantTotal := len(systemrepo.SystemSchemaOperations())
	first := page("")
	if first["repository"] != "kr://kc/system" || first["continuation"] == nil {
		t.Fatalf("unexpected first page: %#v", first)
	}
	if schemas, ok := first["schemas"].([]any); !ok || len(schemas) != 2 {
		t.Fatalf("unexpected first schema page: %#v", first["schemas"])
	}
	row := first["schemas"].([]any)[0].(map[string]any)
	if row["repository"] != nil || row["commit"] != nil || row["fields"] != nil || row["digest"] != nil ||
		row["path"] != nil || row["body"] != nil {
		t.Fatalf("schema list must name entities, not return a schema document: %#v", row)
	}
	if row["objectId"] == "" || row["entity"] == "" || row["description"] == "" {
		t.Fatalf("schema list row missing entity name: %#v", row)
	}
	seen := map[string]bool{}
	current := first
	for {
		if current["coverage"] != nil || current["exhausted"] != nil || current["total"] != nil {
			t.Fatalf("schema page must omit pagination inventions: %#v", current)
		}
		schemas := current["schemas"].([]any)
		if len(schemas) == 0 || len(schemas) > 2 || current["commit"] != first["commit"] {
			t.Fatalf("page did not keep its bound and basis: %#v", current)
		}
		for _, schema := range schemas {
			id := schema.(map[string]any)["objectId"].(string)
			if seen[id] {
				t.Fatalf("pagination repeated Schema %s", id)
			}
			seen[id] = true
		}
		if current["continuation"] == nil {
			break
		}
		current = page(current["continuation"].(string))
	}
	for _, schema := range systemrepo.SystemSchemaOperations() {
		if !seen[string(schema.Address.ObjectID)] {
			t.Errorf("paginated directory omitted %s", schema.Address.ObjectID)
		}
	}
	if len(seen) != wantTotal {
		t.Fatalf("Schema directory contains %d entries, want %d", len(seen), wantTotal)
	}

	status, zero, raw := request(map[string]any{"repository": "kr://kc/system", "limit": 0})
	if status != http.StatusOK || zero["continuation"] != nil || len(zero["schemas"].([]any)) != wantTotal {
		t.Fatalf("schema page limit 0 must mean the default page: status=%d payload=%#v raw=%s", status, zero, raw)
	}
	status, oversized, raw := request(map[string]any{"repository": "kr://kc/system", "limit": 201})
	errObj, _ := oversized["error"].(map[string]any)
	if status != http.StatusBadRequest || errObj["code"] != string(kernel.ErrUsageInvalid) {
		t.Fatalf("schema page limit 201 status=%d payload=%#v raw=%s", status, oversized, raw)
	}
}
