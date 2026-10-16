package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	apphome "kc/home"
	inv "kc/invocation"
	"kc/object"
	"kc/observability"
	"kc/snapshot"
	"kc/snapshot/dataset"
	snapshotservice "kc/snapshot/service"
)

func auditedFileGateway(t *testing.T) (apphome.DeploymentConfig, http.Handler, dataset.ResolvedDataset) {
	t.Helper()
	cfg, configPath := declaredFileGateway(t)
	opened, err := apphome.OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := inv.PickDatasets(opened, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cat.DefineDataset("protocol", 2, []dataset.DatasetSource{
		{Repository: object.SystemRepositoryID, Selector: snapshot.DefaultRef, SubPath: "concepts", Path: dataset.MountPath("docs")},
		{Repository: object.SystemRepositoryID, Selector: snapshot.DefaultRef, File: "README.md", Target: "about/platform.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	pin, err := cat.ResolveDataset("protocol")
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
	t.Cleanup(func() {
		if err := handler.(interface{ Close() error }).Close(); err != nil {
			t.Error(err)
		}
	})
	return cfg, handler, pin
}

func fileEvidenceCall(t *testing.T, handler http.Handler, principal, route, requestID, traceID string, input any, status int) []byte {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, route, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kc-As", principal)
	req.Header.Set("X-Kc-Request-Id", requestID)
	req.Header.Set("X-Kc-Trace-Id", traceID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != status {
		t.Fatalf("%s: %d %s; want %d", route, response.Code, response.Body.String(), status)
	}
	return response.Body.Bytes()
}

func fileEvidenceQuery(t *testing.T, handler http.Handler, query map[string]any) []observability.AccessEvent {
	t.Helper()
	raw := fileEvidenceCall(t, handler, "agent:operator", "/operations/v1/access-log:query", "audit-query", "audit-query-trace", query, http.StatusOK)
	var page struct {
		Entries []observability.AccessEvent `json:"entries"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	return page.Entries
}

func TestFileAccessAuditRecordsActualSourcesByTraceAndRequest(t *testing.T) {
	_, handler, pin := auditedFileGateway(t)
	rawPin, _ := json.Marshal(pin)
	commit := pin.Items[0].Commit
	cases := []struct {
		name, principal, path string
		request               snapshotservice.FileReadRequest
		dataset               bool
	}{
		{"repository", "agent:operator", "concepts/object/definition.md", snapshotservice.FileReadRequest{Repository: string(object.SystemRepositoryID), Commit: string(commit), Path: "concepts/object/definition.md"}, false},
		{"prefix", "agent:reader", "concepts/object/definition.md", snapshotservice.FileReadRequest{FileCoordinate: snapshotservice.FileCoordinate{Dataset: "protocol", Pin: rawPin}, Path: "docs/object/definition.md"}, true},
		{"entry", "agent:reader", "README.md", snapshotservice.FileReadRequest{FileCoordinate: snapshotservice.FileCoordinate{Dataset: "protocol", Pin: rawPin}, Path: "about/platform.md"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for chunk := 0; chunk < 2; chunk++ {
				request := tc.request
				request.Offset = int64(chunk * 4)
				request.Length = 4
				id := fmt.Sprintf("%s-%d", tc.name, chunk)
				fileEvidenceCall(t, handler, tc.principal, "/snapshot/v1/files/file:read", id, "file-task", request, http.StatusOK)
				events := fileEvidenceQuery(t, handler, map[string]any{"requestId": id})
				if len(events) != 1 {
					t.Fatalf("one file chunk must leave exactly one event, got %#v", events)
				}
				event := events[0]
				want := snapshot.FileRef{Repository: object.SystemRepositoryID, Commit: commit, Path: tc.path}
				if event.Identity.Principal != tc.principal || event.Trace.TraceID != "file-task" || event.RequestID != id || event.Action != "file.read" || event.Decision != "ALLOW" || event.Result != "RESOLVED" || len(event.Files) != 1 || event.Files[0].FileRef != want {
					t.Fatalf("wrong actual file evidence: %#v; want %#v", event, want)
				}
				if tc.dataset {
					if event.Dataset != "protocol" || event.PinID != pin.PinID {
						t.Fatalf("lost accepted Dataset basis: %#v", event)
					}
				} else if event.Dataset != "" || event.PinID != "" {
					t.Fatalf("repository read invented Dataset basis: %#v", event)
				}
			}
		})
	}
	events := fileEvidenceQuery(t, handler, map[string]any{"traceId": "file-task"})
	if len(events) != 6 {
		t.Fatalf("trace must return all six chunks once: %#v", events)
	}
	if events := fileEvidenceQuery(t, handler, map[string]any{"traceId": "other-task", "requestId": "repository-0"}); len(events) != 0 {
		t.Fatalf("crossed trace/request: %#v", events)
	}
}

func TestFileAccessAuditFailurePreventsSuccessfulDelivery(t *testing.T) {
	cfg, handler, pin := auditedFileGateway(t)
	if err := os.RemoveAll(filepath.Join(cfg.StateDir, "access")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.StateDir, "access"), []byte("blocked evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	fileEvidenceCall(t, handler, "agent:operator", "/snapshot/v1/files/file:read", "file-not-delivered", "file-failure", map[string]any{"repository": object.SystemRepositoryID, "commit": pin.Items[0].Commit, "path": "README.md", "length": 4}, http.StatusServiceUnavailable)
}
