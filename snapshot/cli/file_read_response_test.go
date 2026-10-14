package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kc/kernel"
	snapshotcli "kc/snapshot/cli"
	"kc/snapshot/client"
	"kc/snapshot/dataset"
)

func TestFileDownloadFailuresDoNotPublishPartialTargets(t *testing.T) {
	for _, failure := range []string{"revoked", "changed-version", "changed-source", "changed-pinned-source", "changed-source-path", "changed-size", "no-progress"} {
		t.Run(failure, func(t *testing.T) {
			pin := dataset.ResolvedKnowledgeSet{SetID: "delivery", Revision: 2, PinID: "fixed", Repositories: map[kernel.RepositoryID]kernel.CommitID{"docs": "commit", "archive": "archive-commit"}}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/snapshot/v1/datasets/delivery/resolve" {
					_ = json.NewEncoder(w).Encode(pin)
					return
				}
				if r.URL.Path != "/snapshot/v1/files/file:read" {
					t.Errorf("unexpected route: %s", r.URL.Path)
					http.Error(w, "unexpected route", http.StatusBadRequest)
					return
				}
				calls++
				var request client.WorkspaceFileReadRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				read := client.WorkspaceFileReadResponse{Pin: pin, File: "file.bin", Path: request.Path, Offset: request.Offset, TotalBytes: (512 << 10) + 1}
				read.Mount = dataset.VirtualMount{Repository: "docs", SubPath: "source", Commit: "commit"}
				if calls == 1 {
					read.Content = bytes.Repeat([]byte{255}, (512 << 10))
				} else {
					read.Content, read.EOF = []byte{0}, true
				}
				switch {
				case failure == "revoked" && calls == 2:
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"error":{"code":"FORBIDDEN","message":"revoked"}}`))
					return
				case failure == "changed-version" && calls == 2:
					read.Pin.Revision++
				case failure == "changed-source" && calls == 2:
					read.Mount.Commit = "another-commit"
				case failure == "changed-pinned-source" && calls == 2:
					read.Mount.Repository, read.Mount.Commit = "archive", "archive-commit"
				case failure == "changed-source-path" && calls == 2:
					read.File = "another-source.bin"
				case failure == "changed-size" && calls == 2:
					read.TotalBytes++
					read.Content = []byte{0, 1}
				case failure == "no-progress":
					read.Content = nil
				}
				_ = json.NewEncoder(w).Encode(read)
			}))
			defer server.Close()
			remote, err := client.New(client.Config{BaseURL: server.URL, Transport: &testTransport{client: server.Client()}})
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			target := filepath.Join(dir, "out.bin")
			_, transferErr := snapshotcli.New(remote, snapshotcli.Context{}).Execute(context.Background(), "files read", map[string]any{"dataset": "delivery", "revision": "2", "path": "file.bin", "out": target}, client.RequestOptions{})
			if transferErr == nil {
				t.Fatal("invalid transfer succeeded")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("failed transfer left partial output: %v %v", entries, err)
			}
			wantCalls := 2
			if failure == "no-progress" {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("test did not reach the intended transfer step: calls=%d want=%d %v", calls, wantCalls, transferErr)
			}
		})
	}
}
