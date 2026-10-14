package cli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	snapshotcli "kc/snapshot/cli"
	"kc/snapshot/client"
	"kc/snapshot/dataset"
)

func cloneRecordingSDK(t *testing.T, read func(http.ResponseWriter, client.WorkspaceFileReadRequest, dataset.ResolvedKnowledgeSet)) (client.Client, *testTransport) {
	t.Helper()
	pin := dataset.ResolvedKnowledgeSet{SetID: "delivery", Revision: 1, PinID: "fixed-publication", Repositories: map[kernel.RepositoryID]kernel.CommitID{"kr://source": "fixed-commit"}, Items: []dataset.DatasetItem{{Kind: dataset.DatasetItemFile, Repository: "kr://source", Commit: "fixed-commit", File: "source.bin", Target: "file.bin"}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer caller-provided" {
			t.Errorf("clone did not use the configured transport: %s %s", request.Method, request.URL.Path)
		}
		switch request.URL.Path {
		case "/snapshot/v1/datasets/delivery/resolve":
			_ = json.NewEncoder(w).Encode(pin)
		case "/snapshot/v1/files/mounts:list":
			_ = json.NewEncoder(w).Encode(client.WorkspaceFileMountsResponse{Pin: pin})
		case "/snapshot/v1/files/tree:list":
			_ = json.NewEncoder(w).Encode(client.WorkspaceFileDirectoryResponse{Pin: pin, Entries: []snapshot.DirectoryEntry{{Name: "file.bin", Kind: "file"}}, Exhausted: true})
		case "/snapshot/v1/files/file:read":
			var input client.WorkspaceFileReadRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			var fixed dataset.ResolvedKnowledgeSet
			if json.Unmarshal(input.Pin, &fixed) != nil || fixed.PinID != pin.PinID || input.Path != "file.bin" {
				t.Errorf("clone file request lost its publication or delivered path: %#v", input)
			}
			read(w, input, pin)
		default:
			t.Errorf("clone used unexpected operation %s", request.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	transport := &testTransport{client: server.Client()}
	sdk, err := client.New(client.Config{BaseURL: server.URL, Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	return sdk, transport
}

func TestModuleCloneMaterializesZeroByteFileAndCountsIt(t *testing.T) {
	sdk, transport := cloneRecordingSDK(t, func(w http.ResponseWriter, input client.WorkspaceFileReadRequest, pin dataset.ResolvedKnowledgeSet) {
		if input.Offset != 0 {
			t.Errorf("empty file requested another range: %d", input.Offset)
		}
		_ = json.NewEncoder(w).Encode(client.WorkspaceFileReadResponse{Pin: pin, Path: input.Path, EOF: true})
	})
	target := filepath.Join(t.TempDir(), "cloned")
	commands := snapshotcli.New(sdk, snapshotcli.Context{})
	flags := map[string]any{"dataset": "delivery", "dir": target}
	output, err := commands.Execute(context.Background(), "dataset clone", flags, client.RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(target, "file.bin"))
	if err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
		t.Fatalf("empty delivered file was not materialized as a zero-byte file: %#v %v", info, err)
	}
	receipt, ok := output.(map[string]any)
	if !ok || receipt["files"] != 1 || receipt["bytes"] != int64(0) || receipt["fileEntries"] != 1 || receipt["pinId"] != "fixed-publication" {
		t.Fatalf("clone did not count the actual empty file: %#v", output)
	}
	calls := transport.calls
	if _, err := commands.Execute(context.Background(), "dataset clone", flags, client.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || transport.calls != calls {
		t.Fatalf("clone overwrote or requested content for an existing target: %d %v", transport.calls, err)
	}
}

func TestModuleCloneNonAdvancingResponseDoesNotCreateAnEmptyFile(t *testing.T) {
	sdk, _ := cloneRecordingSDK(t, func(w http.ResponseWriter, input client.WorkspaceFileReadRequest, pin dataset.ResolvedKnowledgeSet) {
		_ = json.NewEncoder(w).Encode(client.WorkspaceFileReadResponse{Pin: pin, Path: input.Path, EOF: false})
	})
	target := filepath.Join(t.TempDir(), "cloned")
	_, err := snapshotcli.New(sdk, snapshotcli.Context{}).Execute(context.Background(), "dataset clone", map[string]any{"dataset": "delivery", "dir": target}, client.RequestOptions{})
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("non-advancing file read reported successful materialization: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "file.bin")); !os.IsNotExist(err) {
		t.Fatalf("non-advancing response created a misleading empty file: %v", err)
	}
}

func TestModuleCloneReadFailurePreservesPreviouslyWrittenBytes(t *testing.T) {
	sdk, _ := cloneRecordingSDK(t, func(w http.ResponseWriter, input client.WorkspaceFileReadRequest, pin dataset.ResolvedKnowledgeSet) {
		if input.Offset == 0 {
			_ = json.NewEncoder(w).Encode(client.WorkspaceFileReadResponse{Pin: pin, Path: input.Path, Content: []byte("partial"), TotalBytes: 20})
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(kernel.FaultJSON(kernel.Fail(kernel.ErrForbidden, "file access revoked")))
	})
	target := filepath.Join(t.TempDir(), "cloned")
	_, err := snapshotcli.New(sdk, snapshotcli.Context{}).Execute(context.Background(), "dataset clone", map[string]any{"dataset": "delivery", "dir": target}, client.RequestOptions{})
	if kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("later file-read failure lost its verdict: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(target, "file.bin"))
	if err != nil || string(content) != "partial" {
		t.Fatalf("clone changed its established partial-output behavior: %q %v", content, err)
	}
}
