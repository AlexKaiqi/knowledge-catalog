package client_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	snapshotclient "kc/snapshot/client"
)

func TestRepositoryFileClientKeepsLogicalFixedCoordinates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/snapshot/v1/files/file:read" || r.Header.Get("X-Kc-Request-Id") != "resource-read" {
			t.Errorf("raw file request changed route or correlation: %s %s", r.Method, r.URL.Path)
		}
		var request snapshotclient.WorkspaceFileReadRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Repository != "kr://team/assets" || request.Commit != "fixed-commit" || request.Path != "media/image.png" || request.Offset != 4 || request.Length != 3 || request.Dataset != "" || len(request.Pin) != 0 {
			t.Errorf("logical resource coordinate was translated or lost: %#v", request)
		}
		_ = json.NewEncoder(w).Encode(snapshotclient.RepositoryFileReadResponse{
			Repository: "kr://team/assets", Commit: "fixed-commit", Path: "media/image.png",
			Offset: 4, TotalBytes: 7, EOF: true, Content: []byte{0, 255, 1},
		})
	}))
	defer server.Close()
	var response snapshotclient.RepositoryFileReadResponse
	err := metadataClient(t, server).WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
		Repository: "kr://team/assets", Commit: "fixed-commit", Path: "media/image.png", Offset: 4, Length: 3,
	}, snapshotclient.RequestOptions{RequestID: "resource-read"}, &response)
	if err != nil || response.Repository != "kr://team/assets" || response.Commit != "fixed-commit" || response.Path != "media/image.png" || response.Offset != 4 || response.TotalBytes != 7 || !response.EOF || !bytes.Equal(response.Content, []byte{0, 255, 1}) {
		t.Fatalf("raw fixed file response changed: %#v %v", response, err)
	}
}
