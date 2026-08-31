package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/catalog"
)

func TestWorkspaceFileGatewayPagesDirectChildrenAndReadsFixedRange(t *testing.T) {
	home := t.TempDir()
	projectRepo := "kr://acme/docs"
	catalogID := "kr://acme/catalog"
	mustWorkspaceFSRun(t, home, "init", "--catalog", catalogID)
	mustWorkspaceFSRun(t, home, "repo-add", "--repo", projectRepo)
	mustWorkspaceFSRun(t, home, "define-workspace", "--workspace", "agent", "--revision", "1",
		"--source", projectRepo+"=refs/heads/main@knowledge@shared")
	mustRawTreeWrite(t, home, projectRepo, "files", "shared/a.txt", "alpha")
	mustRawTreeWrite(t, home, projectRepo, "nested", "shared/nested/b.txt", "bravo")
	for _, args := range [][]string{
		{"admin", "grant", "add", "--principal", "agent:test", "--action", "workspace.resolve", "--catalog", catalogID, "--workspace", "agent"},
		{"admin", "grant", "add", "--principal", "agent:test", "--action", "file.read", "--repo", projectRepo},
	} {
		if result := runWithTelemetryMode(append([]string{"--home", home}, args...), nil, true); result.Status != 0 {
			t.Fatalf("grant failed: %s", result.Stdout)
		}
	}
	server := httptest.NewServer(HTTPHandler(home))
	defer server.Close()

	mounts := postWorkspaceFiles(t, server.URL+"/workspace-files/v1/mounts:list", map[string]any{
		"catalog": catalogID, "workspace": "agent",
	})
	pin := mounts["pin"]
	mountList := mounts["mounts"].([]any)
	if len(mountList) != 1 || mountList[0].(map[string]any)["path"] != "knowledge" {
		t.Fatalf("mounts = %#v", mounts)
	}
	first := postWorkspaceFiles(t, server.URL+"/workspace-files/v1/tree:list", map[string]any{
		"catalog": catalogID, "workspace": "agent", "pin": pin, "mountPath": "knowledge", "limit": 1,
	})
	entries := first["entries"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["name"] != "a.txt" || first["continuation"] == nil {
		t.Fatalf("first directory page = %#v", first)
	}
	second := postWorkspaceFiles(t, server.URL+"/workspace-files/v1/tree:list", map[string]any{
		"catalog": catalogID, "workspace": "agent", "pin": pin, "mountPath": "knowledge", "limit": 1,
		"continuation": first["continuation"],
	})
	secondEntry := second["entries"].([]any)[0].(map[string]any)
	if secondEntry["name"] != "nested" || secondEntry["kind"] != "directory" || second["exhausted"] != true {
		t.Fatalf("second directory page = %#v", second)
	}
	read := postWorkspaceFiles(t, server.URL+"/workspace-files/v1/file:read", map[string]any{
		"catalog": catalogID, "workspace": "agent", "pin": pin, "mountPath": "knowledge", "file": "a.txt", "offset": 1, "length": 3,
	})
	content, err := io.ReadAll(bytes.NewReader(mustDecodeBase64(t, read["content"].(string))))
	if err != nil || string(content) != "lph" || read["eof"] != false {
		t.Fatalf("range read = %#v, %q, %v", read, content, err)
	}

	// A directory continuation is bound to its mount commit and directory.
	bad := serviceRequest(t, server.URL+"/workspace-files/v1/tree:list", map[string]any{
		"catalog": catalogID, "workspace": "agent", "pin": pin, "mountPath": "knowledge", "directory": "nested", "limit": 1,
		"continuation": first["continuation"],
	})
	if bad.Code != http.StatusConflict {
		t.Fatalf("continuation replay status = %d body %s", bad.Code, bad.Body.String())
	}

	var resolved catalog.ResolvedWorkspace
	raw, _ := json.Marshal(pin)
	if err := json.Unmarshal(raw, &resolved); err != nil || resolved.PinID == "" {
		t.Fatalf("invalid returned pin: %#v %v", pin, err)
	}
	metrics, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer metrics.Body.Close()
	metricBody, _ := io.ReadAll(metrics.Body)
	metricText := string(metricBody)
	for _, want := range []string{"kc_vfs_transfer_size_bytes_sum", "kc_vfs_directory_entry_count_sum", "kc_identity_requests_total", `kc_principal_kind="agent"`} {
		if !strings.Contains(metricText, want) {
			t.Fatalf("Workspace File telemetry missing %q:\n%s", want, metricText)
		}
	}
}

func postWorkspaceFiles(t *testing.T, url string, body any) map[string]any {
	t.Helper()
	response := serviceRequest(t, url, body)
	if response.Code != http.StatusOK {
		t.Fatalf("POST %s: status %d body %s", url, response.Code, response.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func serviceRequest(t *testing.T, url string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Kc-As", "agent:test")
	response := httptest.NewRecorder()
	actual, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer actual.Body.Close()
	response.Code = actual.StatusCode
	_, _ = io.Copy(response.Body, actual.Body)
	return response
}

func mustDecodeBase64(t *testing.T, value string) []byte {
	t.Helper()
	var out []byte
	if err := json.Unmarshal([]byte(`"`+value+`"`), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
