package client_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	snapshotclient "kc/snapshot/client"
)

func TestStandaloneDatasetRequestCompressionAndExactJSON(t *testing.T) {
	value := strings.Repeat("v", 600<<10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/snapshot/v1/datasets" || r.Header.Get("Content-Encoding") != "gzip" || r.Header.Get("X-Kc-Request-Id") != "dataset-publication" {
			t.Errorf("request shape changed: %s %s %#v", r.Method, r.URL.Path, r.Header)
		}
		reader, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer reader.Close()
		var body snapshotclient.KnowledgeSetRequest
		if json.NewDecoder(reader).Decode(&body) != nil || body.Metadata["large"] != value {
			t.Error("compressed Dataset definition lost metadata")
		}
		_, _ = w.Write([]byte(`{"revision":1,"size":9007199254740993}`))
	}))
	defer server.Close()
	sdk := metadataClient(t, server)
	var result map[string]any
	err := sdk.SnapshotService().DefineKnowledgeSet(context.Background(), snapshotclient.KnowledgeSetRequest{Dataset: "delivery", Revision: 1, Metadata: map[string]string{"large": value}}, snapshotclient.RequestOptions{RequestID: "dataset-publication"}, &result)
	if err != nil {
		t.Fatal(err)
	}
	if result["size"] != json.Number("9007199254740993") {
		t.Fatalf("response lost exact integer: %#v", result)
	}
}

func TestStandaloneDatasetFileRangePreservesCoordinatesAndBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/snapshot/v1/files/file:read" {
			t.Errorf("file API changed: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		var body snapshotclient.WorkspaceFileReadRequest
		if json.Unmarshal(raw, &body) != nil || body.Dataset != "delivery" || body.Path != "picked/file.bin" || body.Offset != 7 || body.Length != 3 || string(body.Pin) != `{"setId":"delivery","revision":1}` {
			t.Errorf("fixed file coordinates changed: %s", raw)
		}
		_, _ = w.Write([]byte(`{"pin":{"setId":"delivery","revision":1},"file":"file.bin","path":"picked/file.bin","offset":7,"totalBytes":10,"eof":true,"content":"AAEC"}`))
	}))
	defer server.Close()
	var result snapshotclient.WorkspaceFileReadResponse
	err := metadataClient(t, server).WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{WorkspaceFileCoordinate: snapshotclient.WorkspaceFileCoordinate{Dataset: "delivery", Pin: json.RawMessage(`{"setId":"delivery","revision":1}`)}, Path: "picked/file.bin", Offset: 7, Length: 3}, snapshotclient.RequestOptions{}, &result)
	if err != nil || string(result.Content) != string([]byte{0, 1, 2}) || result.Pin.Revision != 1 || !result.EOF {
		t.Fatalf("fixed byte delivery changed: %#v %v", result, err)
	}
}
