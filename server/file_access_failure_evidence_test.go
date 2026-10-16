package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/kernel"
	"kc/object"
	"kc/observability"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func singleFileFailureAuditEvent(t *testing.T, handler http.Handler, requestID, principal string) observability.AccessEvent {
	t.Helper()
	events := fileEvidenceQuery(t, handler, map[string]any{"requestId": requestID})
	if len(events) != 1 {
		t.Fatalf("one file request must leave one actual access event: %#v", events)
	}
	event := events[0]
	if event.Identity.Principal != principal || event.RequestID != requestID || event.Trace.TraceID != "file-failure-task" || event.Action != "file.read" {
		t.Fatalf("file evidence lost its caller or request identity: %#v", event)
	}
	return event
}

func assertFileAuditFailure(t *testing.T, raw []byte, event observability.AccessEvent, code kernel.ErrorCode, decision string) {
	t.Helper()
	var response struct {
		Error struct{ Code kernel.ErrorCode }
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != code || event.Error["code"] != string(code) || event.Result != "ERROR" || event.Decision != decision {
		t.Fatalf("file failure changed between response and audit: response=%#v event=%#v want=%s/%s", response, event, code, decision)
	}
}

func TestFileAccessAuditRecordsRepositoryFailures(t *testing.T) {
	_, handler, pin := auditedFileGateway(t)
	for _, tc := range []struct {
		name   string
		commit kernel.CommitID
		path   string
		code   kernel.ErrorCode
	}{
		{"missing-version", "missing-commit", "README.md", kernel.ErrVersionUnresolved},
		{"missing-file", pin.Repositories[object.SystemRepositoryID], "missing-material.md", kernel.ErrObjectRefUnresolved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := map[string]any{"repository": object.SystemRepositoryID, "commit": tc.commit, "path": tc.path}
			raw := fileEvidenceCall(t, handler, "agent:operator", "/snapshot/v1/files/file:read", tc.name, "file-failure-task", request, http.StatusNotFound)
			event := singleFileFailureAuditEvent(t, handler, tc.name, "agent:operator")
			assertFileAuditFailure(t, raw, event, tc.code, "ALLOW")
			want := snapshot.FileRef{Repository: object.SystemRepositoryID, Commit: tc.commit, Path: tc.path}
			if len(event.Files) != 1 || event.Files[0].FileRef != want || event.Dataset != "" || event.PinID != "" || len(event.Snapshots) != 0 {
				t.Fatalf("failed raw read lost its attempted target or invented a Dataset: %#v; want %#v", event, want)
			}
		})
	}
}

func revokeFileAuditReader(t *testing.T, handler http.Handler) {
	t.Helper()
	request := httptest.NewRequest(http.MethodDelete, "/admin/v1/grants/file-reader", nil)
	request.Header.Set("X-Kc-As", "agent:operator")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("revoke file consumer: %d %s", response.Code, response.Body.String())
	}
}

func TestFileAccessAuditRejectedDatasetNeverTrustsCallerPin(t *testing.T) {
	for _, tc := range []struct {
		name, decision string
		status         int
		code           kernel.ErrorCode
		revoke         bool
	}{
		{"revoked-dataset", "DENY", http.StatusForbidden, kernel.ErrForbidden, true},
		{"forged-pin", "ALLOW", http.StatusBadRequest, kernel.ErrUsageInvalid, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, handler, pin := auditedFileGateway(t)
			if tc.revoke {
				revokeFileAuditReader(t, handler)
			} else {
				pin.Repositories["kr://forged/secret"] = "forged-commit"
				pin.Items = append(pin.Items, dataset.DatasetItem{Kind: dataset.DatasetItemFile, Repository: "kr://forged/secret", Commit: "forged-commit", File: "hidden.txt", Target: "hidden.txt"})
				pin.PinID = "forged-pin-id"
			}
			request := map[string]any{"dataset": "protocol", "pin": pin, "path": "docs/object/definition.md"}
			raw := fileEvidenceCall(t, handler, "agent:reader", "/snapshot/v1/files/file:read", tc.name, "file-failure-task", request, tc.status)
			event := singleFileFailureAuditEvent(t, handler, tc.name, "agent:reader")
			assertFileAuditFailure(t, raw, event, tc.code, tc.decision)
			if event.Dataset != "protocol" || event.PinID != "" || len(event.Files) != 0 || len(event.Snapshots) != 0 {
				t.Fatalf("unaccepted Dataset read claimed a source or accepted caller pin: %#v", event)
			}
		})
	}
}

func TestFileAccessAuditSemanticViewUsesPinnedSnapshot(t *testing.T) {
	_, handler, _ := auditedFileGateway(t)
	raw := fileEvidenceCall(t, handler, "agent:reader", "/snapshot/v1/files/mounts:list", "semantic-mounts", "file-failure-task", map[string]any{"dataset": "protocol", "view": "semantic"}, http.StatusOK)
	var mounts workspaceFileMountsResponse
	if err := json.Unmarshal(raw, &mounts); err != nil {
		t.Fatal(err)
	}
	if len(mounts.Mounts) != 1 || mounts.Pin.PinID == "" {
		t.Fatalf("semantic setup did not provide one accepted mount: %#v", mounts)
	}
	for _, tc := range []struct {
		name, file string
		status     int
		code       kernel.ErrorCode
	}{
		{"semantic-read", "_meta/repository.yaml", http.StatusOK, ""},
		{"semantic-missing", "_meta/missing.yaml", http.StatusNotFound, kernel.ErrObjectRefUnresolved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mount := mounts.Mounts[0]
			request := map[string]any{"dataset": "protocol", "view": "semantic", "pin": mounts.Pin, "mountPath": mount.Path, "file": tc.file}
			raw := fileEvidenceCall(t, handler, "agent:reader", "/snapshot/v1/files/file:read", tc.name, "file-failure-task", request, tc.status)
			event := singleFileFailureAuditEvent(t, handler, tc.name, "agent:reader")
			if tc.code != "" {
				assertFileAuditFailure(t, raw, event, tc.code, "ALLOW")
			} else if event.Result != "RESOLVED" || event.Decision != "ALLOW" || len(event.Error) != 0 {
				t.Fatalf("semantic success recorded as failure: %#v", event)
			}
			want := observability.SnapshotAccess{Repository: mount.Repository, Commit: mount.Commit}
			if event.Dataset != "protocol" || event.PinID != mounts.Pin.PinID || len(event.Files) != 0 || len(event.Snapshots) != 1 || event.Snapshots[0] != want {
				t.Fatalf("semantic projection invented a source file or lost its fixed snapshot: %#v; want %#v", event, want)
			}
		})
	}
}

func TestFileAccessAuditPreservesCommitScopedGrant(t *testing.T) {
	_, handler, pin := auditedFileGateway(t)
	commit := pin.Repositories[object.SystemRepositoryID]
	raw := fileEvidenceCall(t, handler, "agent:operator", "/admin/v1/grants", "grant-fixed-file", "file-failure-task", map[string]any{
		"principal": "agent:fixed-reader", "actions": []string{"file.read"}, "repository": object.SystemRepositoryID, "ref": commit,
	}, http.StatusOK)
	var grant struct{ ID string }
	if err := json.Unmarshal(raw, &grant); err != nil || grant.ID == "" {
		t.Fatalf("fixed commit grant was not accepted: %s; %v", raw, err)
	}
	request := map[string]any{"repository": object.SystemRepositoryID, "commit": commit, "path": "README.md"}
	fileEvidenceCall(t, handler, "agent:fixed-reader", "/snapshot/v1/files/file:read", "commit-granted", "file-failure-task", request, http.StatusOK)
	event := singleFileFailureAuditEvent(t, handler, "commit-granted", "agent:fixed-reader")
	if event.RuleID != grant.ID || event.Decision != "ALLOW" || event.Result != "RESOLVED" {
		t.Fatalf("read audit lost the commit-scoped authorization rule: %#v; want rule %s", event, grant.ID)
	}
	request["commit"] = "not-authorized-commit"
	raw = fileEvidenceCall(t, handler, "agent:fixed-reader", "/snapshot/v1/files/file:read", "commit-denied", "file-failure-task", request, http.StatusForbidden)
	event = singleFileFailureAuditEvent(t, handler, "commit-denied", "agent:fixed-reader")
	assertFileAuditFailure(t, raw, event, kernel.ErrForbidden, "DENY")
	if event.RuleID != "" {
		t.Fatalf("denied commit borrowed another commit's grant: %#v", event)
	}
}
