package cli_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kc/client"
	"kc/kernel"
	serv "kc/server"
)

func TestDatasetConsoleManagementHistoryAndByteContract(t *testing.T) {
	f := newDirectoryDelivery(t)
	original := f.mounts(t, nil).Pin
	if err := f.publish(3, directoryDeliverySources("latest/policies", "latest/guides", "latest/metrics")); err != nil {
		t.Fatal(err)
	}
	endpoint := "/snapshot/v1/datasets/delivery"
	get := func(query, principal string) (int, map[string]any) {
		t.Helper()
		request, err := http.NewRequest(http.MethodGet, f.serverURL+endpoint+query, nil)
		if err != nil {
			t.Fatal(err)
		}
		if principal != "" {
			request.Header.Set("X-Kc-As", principal)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var value map[string]any
		if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, value
	}
	status, view := get("?revision=1", "agent:publisher")
	if status != http.StatusOK || view["revision"] != float64(1) || view["servingRevision"] != float64(3) || !reflect.DeepEqual(view["versions"], []any{float64(1), float64(3)}) {
		t.Fatalf("history must name accepted releases, not fill revision gaps: %d %#v", status, view)
	}
	sources := view["sources"].([]any)
	if sources[0].(map[string]any)["commit"] != string(original.Repositories[deliveryDocs]) {
		t.Fatalf("historical source changed: %#v", sources)
	}
	status, _ = get("?revision=1", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("historical metadata bypassed authentication: %d", status)
	}
	status, _ = get("?revision=1", "agent:consumer")
	if status != http.StatusOK {
		t.Fatalf("authenticated public Catalog discovery was tightened: %d", status)
	}
	status, firstPage := get("?limit=1", "agent:publisher")
	continuation, ok := firstPage["continuation"].(string)
	if status != http.StatusOK || !reflect.DeepEqual(firstPage["versions"], []any{float64(3)}) || !ok || continuation == "" {
		t.Fatalf("history first page did not use latest accepted release: %d %#v", status, firstPage)
	}
	status, secondPage := get("?revision=1&limit=1&continuation="+url.QueryEscape(continuation), "agent:publisher")
	if status != http.StatusOK || !reflect.DeepEqual(secondPage["versions"], []any{float64(1)}) || secondPage["continuation"] != nil {
		t.Fatalf("history continuation fabricated the missing v2: %d %#v", status, secondPage)
	}
	for _, query := range []string{"?limit=-1", "?limit=bad", "?limit=201", "?continuation=bad"} {
		status, value := get(query, "agent:publisher")
		fault, ok := value["error"].(map[string]any)
		if status != http.StatusBadRequest || !ok || fault["code"] != string(kernel.ErrUsageInvalid) {
			t.Fatalf("history query %s: %d %#v", query, status, value)
		}
	}
	for _, revision := range []string{"0", "-1", "abc", "999"} {
		status, value := get("?revision="+revision, "agent:publisher")
		code := string(kernel.ErrUsageInvalid)
		if revision == "999" {
			code = string(kernel.ErrVersionUnresolved)
		}
		fault, ok := value["error"].(map[string]any)
		if status < 400 || !ok || fault["code"] != code {
			t.Fatalf("revision %s: %d %#v", revision, status, value)
		}
	}
	payload, err := json.Marshal(map[string]any{"dataset": "delivery", "pin": original, "view": "repository", "path": "reference/policies/README.md", "length": 65536})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, f.serverURL+"/snapshot/v1/files/file:read", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Kc-As", "agent:consumer")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var file struct {
		Content string `json:"content"`
		EOF     bool   `json:"eof"`
	}
	if err := json.NewDecoder(response.Body).Decode(&file); err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(file.Content)
	if err != nil || response.StatusCode != 200 || !file.EOF || string(raw) != "policy v1\n" {
		t.Fatalf("preview contract lost frozen bytes: %d %#v %v", response.StatusCode, file, err)
	}
	if err := f.publish(4, directoryDeliverySources("latest/policies", "latest/guides", "latest/metrics")); err != nil {
		t.Fatal(err)
	}
	status, stale := get("?continuation="+url.QueryEscape(continuation), "agent:publisher")
	fault, ok := stale["error"].(map[string]any)
	if status != http.StatusBadRequest || !ok || fault["code"] != string(kernel.ErrUsageInvalid) {
		t.Fatalf("history pagination silently switched publication basis: %d %#v", status, stale)
	}
	if err := f.operator.SnapshotService().RetireKnowledgeSet(context.Background(), "delivery", client.RequestOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	status, view = get("?revision=1", "agent:publisher")
	if status != http.StatusOK || view["retired"] != true {
		t.Fatalf("old metadata must inherit current retirement: %d %#v", status, view)
	}
}

func TestRegistryAuditRequiresCurrentGrant(t *testing.T) {
	f := newDirectoryDelivery(t)
	raw, err := json.Marshal(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "private-history.json")
	if err := os.WriteFile(config, raw, 0600); err != nil {
		t.Fatal(err)
	}
	handler, err := serv.HTTPHandlerFromConfig(config, serv.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer handler.(interface{ Close() error }).Close()
	server := httptest.NewServer(handler)
	defer server.Close()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/snapshot/v1/audit", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Kc-As", "agent:consumer")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var value map[string]any
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	fault, ok := value["error"].(map[string]any)
	if response.StatusCode != http.StatusForbidden || !ok || fault["code"] != string(kernel.ErrForbidden) || value["sources"] != nil {
		t.Fatalf("registry audit bypassed current registry.audit.read: %d %#v", response.StatusCode, value)
	}
}
