package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kc/kernel"
	snapshotcli "kc/snapshot/cli"
	"kc/snapshot/client"
)

func TestRepositoryFileDownloadUsesFixedCoordinates(t *testing.T) {
	content := bytes.Repeat([]byte{0, 255, 128, '\n', 'a'}, 120000)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/snapshot/v1/files/file:read" || r.Header.Get("Authorization") != "Bearer caller-provided" {
			t.Errorf("download bypassed the authenticated file service: %s", r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["repository"] != "kr://assets/pictures" || request["commit"] != "image-version" || request["path"] != "media/image.png" {
			t.Errorf("resource coordinates changed: %#v", request)
		}
		if value := request["dataset"]; value != nil && value != "" {
			t.Errorf("repository download invented a Dataset: %#v", request)
		}
		offset, _ := request["offset"].(float64)
		length, _ := request["length"].(float64)
		start, end := int(offset), min(int(offset+length), len(content))
		calls++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"repository": request["repository"], "commit": request["commit"], "path": request["path"],
			"offset": start, "totalBytes": len(content), "eof": end == len(content), "content": content[start:end],
		})
	}))
	defer server.Close()
	sdk, err := client.New(client.Config{BaseURL: server.URL, Transport: &testTransport{client: server.Client()}})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "image.png")
	flags := map[string]any{"repo": "kr://assets/pictures", "commit": "image-version", "path": "media/image.png", "out": target}
	executor := snapshotcli.New(sdk, snapshotcli.Context{})
	result, err := executor.Execute(context.Background(), "files read", flags, client.RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(target)
	want := map[string]any{"repository": "kr://assets/pictures", "commit": "image-version", "path": "media/image.png", "out": target, "bytes": int64(len(content))}
	if err != nil || !bytes.Equal(actual, content) || !reflect.DeepEqual(result, want) || calls != 2 {
		t.Fatalf("download changed bytes or fixed receipt: calls=%d result=%#v err=%v", calls, result, err)
	}
	if _, err := executor.Execute(context.Background(), "files read", flags, client.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || calls != 2 {
		t.Fatalf("existing target must fail before a new request: %d %v", calls, err)
	}
}

func TestRepositoryFileInputRequiresOneExplicitFixedScope(t *testing.T) {
	executor := snapshotcli.New(nil, snapshotcli.Context{})
	for _, flags := range []map[string]any{
		{"repo": "", "dataset": "delivery", "path": "media/image.png"},
		{"repo": " ", "commit": "image-version", "path": "media/image.png"},
		{"repo": "kr://assets/pictures", "path": "media/image.png"},
		{"commit": "image-version", "path": "media/image.png"},
		{"repo": "kr://assets/pictures", "commit": "image-version", "path": "../private.png"},
		{"repo": "kr://assets/pictures", "commit": "image-version", "path": "media/image.png", "dataset": "delivery"},
		{"repo": "kr://assets/pictures", "commit": "image-version", "path": "media/image.png", "revision": "2"},
		{"dataset": "delivery", "commit": "image-version", "path": "media/image.png"},
	} {
		if err := executor.Validate("files read", flags); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("ambiguous or incomplete file coordinates accepted: %#v %v", flags, err)
		}
	}
}

func TestRepositoryDownloadRejectsDriftAndRevocationWithoutPublishing(t *testing.T) {
	for _, failure := range []string{"repository", "commit", "path", "size", "revoked", "no-progress"} {
		t.Run(failure, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if failure == "revoked" && calls == 2 {
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"error":{"code":"FORBIDDEN","message":"revoked"}}`))
					return
				}
				response := map[string]any{"repository": "kr://assets/pictures", "commit": "fixed", "path": "image.png", "offset": 0,
					"totalBytes": (512 << 10) + 1, "content": bytes.Repeat([]byte{255}, 512<<10), "eof": false}
				if calls == 2 {
					response["offset"], response["content"], response["eof"] = 512<<10, []byte{0}, true
					switch failure {
					case "repository", "commit", "path":
						response[failure] = "changed"
					case "size":
						response["totalBytes"], response["content"] = (512<<10)+2, []byte{0, 1}
					case "no-progress":
						response["content"], response["eof"] = []byte{}, false
					}
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			sdk, err := client.New(client.Config{BaseURL: server.URL, Transport: &testTransport{client: server.Client()}})
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			_, err = snapshotcli.New(sdk, snapshotcli.Context{}).Execute(context.Background(), "files read",
				map[string]any{"repo": "kr://assets/pictures", "commit": "fixed", "path": "image.png", "out": filepath.Join(dir, "image.png")}, client.RequestOptions{})
			want := kernel.ErrPreconditionFailed
			if failure == "revoked" {
				want = kernel.ErrForbidden
			}
			if kernel.CodeOf(err) != want || calls != 2 {
				t.Fatalf("invalid transfer must fail at the second block: calls=%d error=%v", calls, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("failed transfer left partial output: %v %v", entries, err)
			}
		})
	}
}
