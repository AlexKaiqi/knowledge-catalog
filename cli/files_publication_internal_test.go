package cli

import (
	"bytes"
	"encoding/json"
	snapshotclient "kc/snapshot/client"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"kc/kernel"
	"kc/snapshot/dataset"
)

func TestFileDownloadDoesNotReplaceTargetCreatedDuringTransfer(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	dir := t.TempDir()
	target := filepath.Join(dir, "out.bin")
	concurrentContent := []byte("created by another writer during the download")
	pin := dataset.ResolvedKnowledgeSet{SetID: "delivery", Revision: 2, PinID: "fixed", Repositories: map[kernel.RepositoryID]kernel.CommitID{"docs": "commit"}}
	var calls atomic.Int32
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
		call := calls.Add(1)
		var request snapshotclient.WorkspaceFileReadRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode read request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		read := snapshotclient.WorkspaceFileReadResponse{Pin: pin, Path: request.Path, Offset: request.Offset, TotalBytes: (512 << 10) + 1}
		read.Mount = dataset.VirtualMount{Repository: "docs", Commit: "commit"}
		if call == 1 {
			read.Content = bytes.Repeat([]byte{255}, (512 << 10))
		} else {
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), ".kc-file-") {
				t.Errorf("transfer has no isolated temporary file: %v %v", entries, err)
				http.Error(w, "missing temporary file", http.StatusInternalServerError)
				return
			}
			info, err := entries[0].Info()
			if err != nil || info.Size() != (512<<10) {
				t.Errorf("transfer did not stage the first block: %v %v", info, err)
				http.Error(w, "missing first block", http.StatusInternalServerError)
				return
			}
			other, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				t.Errorf("concurrent target creation: %v", err)
				http.Error(w, "target creation failed", http.StatusInternalServerError)
				return
			}
			_, writeErr := other.Write(concurrentContent)
			closeErr := other.Close()
			if writeErr != nil || closeErr != nil {
				t.Errorf("concurrent writer failed: %v %v", writeErr, closeErr)
				http.Error(w, "target write failed", http.StatusInternalServerError)
				return
			}
			read.Content, read.EOF = []byte{0}, true
		}
		_ = json.NewEncoder(w).Encode(read)
	}))
	defer server.Close()

	result := Run([]string{"files", "read", "--dataset", "delivery", "--revision", "2", "--path", "file.bin", "--out", target, "--server", server.URL, "--as", "consumer"})
	if result.Status == 0 || calls.Load() != 2 || !strings.Contains(result.Stdout, "PRECONDITION_FAILED") {
		t.Fatalf("concurrent publication was not refused after a complete transfer: calls=%d result=%#v", calls.Load(), result)
	}
	actual, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(actual, concurrentContent) {
		t.Fatalf("download replaced the concurrent writer's file: %q %v", actual, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(target) {
		t.Fatalf("refused publication left temporary bytes: %v %v", entries, err)
	}
}

func TestEmptyDeliveredFileProducesEmptyBase64Content(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	pin := dataset.ResolvedKnowledgeSet{SetID: "delivery", Revision: 2, PinID: "fixed", Repositories: map[kernel.RepositoryID]kernel.CommitID{"docs": "commit"}}
	var calls atomic.Int32
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
		calls.Add(1)
		// A zero-length source can legitimately return a nil byte slice.
		read := snapshotclient.WorkspaceFileReadResponse{Pin: pin, Path: "empty.bin", EOF: true}
		read.Mount = dataset.VirtualMount{Repository: "docs", Commit: "commit"}
		_ = json.NewEncoder(w).Encode(read)
	}))
	defer server.Close()

	result := Run([]string{"files", "read", "--dataset", "delivery", "--revision", "2", "--path", "empty.bin", "--server", server.URL, "--as", "consumer"})
	if result.Status != 0 || calls.Load() != 1 {
		t.Fatalf("empty file read failed: calls=%d result=%#v", calls.Load(), result)
	}
	var output map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.Stdout), &output); err != nil {
		t.Fatalf("empty file stdout is not JSON: %s %v", result.Stdout, err)
	}
	if string(output["content"]) != `""` || string(output["totalBytes"]) != "0" || string(output["offset"]) != "0" || string(output["eof"]) != "true" {
		t.Fatalf("empty file lost its zero-byte content/range: %s", result.Stdout)
	}
}
