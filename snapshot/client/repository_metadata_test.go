package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"kc/kernel"
	snapshotclient "kc/snapshot/client"
)

func metadataClient(t *testing.T, server *httptest.Server) snapshotclient.Client {
	t.Helper()
	kc, err := snapshotclient.New(snapshotclient.Config{BaseURL: server.URL, Transport: testTransport{http: server.Client()}})
	if err != nil {
		t.Fatal(err)
	}
	return kc
}

func TestRepositoryMetadataClientPreservesNativeShapesAndCorrelation(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/api/v1/repositories/native%2Fname/metadata" || r.Header.Get("X-Kc-Request-Id") != "metadata-operation" {
			t.Errorf("native path or correlation changed: %s %#v", r.URL.EscapedPath(), r.Header)
		}
		methods = append(methods, r.Method)
		var body map[string]json.RawMessage
		switch r.Method {
		case http.MethodGet:
			write := `{"description":"简述","owner":"arbitrary value","empty":""}`
			_, _ = w.Write([]byte(write))
		case http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			var metadata map[string]string
			if json.Unmarshal(body["metadata"], &metadata) != nil || !reflect.DeepEqual(metadata, map[string]string{"description": "简述", "custom": ""}) || len(body) != 1 {
				t.Errorf("native set shape changed: %#v", body)
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			var keys []string
			if json.Unmarshal(body["keys"], &keys) != nil || !reflect.DeepEqual(keys, []string{"description", "custom"}) || len(body) != 1 {
				t.Errorf("native delete shape changed: %#v", body)
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	snapshot := metadataClient(t, server).SnapshotService()
	options := snapshotclient.RequestOptions{RequestID: "metadata-operation"}
	metadata, err := snapshot.RepositoryMetadata(context.Background(), "native/name", options)
	if err != nil || !reflect.DeepEqual(metadata, map[string]string{"description": "简述", "owner": "arbitrary value", "empty": ""}) {
		t.Fatalf("native metadata response = %#v %v", metadata, err)
	}
	if err := snapshot.SetRepositoryMetadata(context.Background(), "native/name", map[string]string{"description": "简述", "custom": ""}, options); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.DeleteRepositoryMetadata(context.Background(), "native/name", []string{"description", "custom"}, options); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(methods, []string{"GET", "POST", "DELETE"}) {
		t.Fatalf("metadata request was replayed: %#v", methods)
	}
}

func TestRepositoryMetadataNativeClientKeepsLargeBodyUncompressed(t *testing.T) {
	value := strings.Repeat("x", 600<<10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") != "" {
			t.Error("native API received KC facade compression")
		}
		raw, err := io.ReadAll(r.Body)
		var body struct {
			Metadata map[string]string `json:"metadata"`
		}
		if err != nil || json.Unmarshal(raw, &body) != nil || body.Metadata["large"] != value {
			t.Error("large native JSON changed")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if err := metadataClient(t, server).SnapshotService().SetRepositoryMetadata(context.Background(), "native-name", map[string]string{"large": value}, snapshotclient.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryMetadataNativeErrorsPreserveRecoveryClass(t *testing.T) {
	for _, row := range []struct {
		status int
		body   string
		code   kernel.ErrorCode
	}{
		{403, `{"message":"product grant denied","code":"FORBIDDEN"}`, kernel.ErrForbidden},
		{409, `{"message":"metadata dependency unavailable","code":"PRECONDITION_FAILED"}`, kernel.ErrPreconditionFailed},
		{400, `{"message":"invalid metadata value"}`, kernel.ErrUsageInvalid},
		{401, `{"message":"native proof rejected"}`, kernel.ErrUnauthenticated},
		{403, `{"message":"native policy denied"}`, kernel.ErrForbidden},
		{404, `{"message":"repository not found"}`, kernel.ErrPreconditionFailed},
		{503, `{"message":"native unavailable"}`, kernel.ErrTemporaryUnavailable},
		{502, `not JSON`, kernel.ErrTemporaryUnavailable},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(row.status)
			_, _ = w.Write([]byte(row.body))
		}))
		_, err := metadataClient(t, server).SnapshotService().RepositoryMetadata(context.Background(), "native-name", snapshotclient.RequestOptions{})
		server.Close()
		if kernel.CodeOf(err) != row.code {
			t.Fatalf("native error %d %s = %v", row.status, row.body, err)
		}
	}
}
