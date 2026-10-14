package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kc/internal/testkit"
)

func TestKnowledgeSetFSPublicCommandAndUsageSurface(t *testing.T) {
	for _, command := range []string{"", "help", "--help", "-h"} {
		var stdout, stderr bytes.Buffer
		argv := []string{}
		if command != "" {
			argv = []string{command}
		}
		if status := RunKnowledgeSetFS(argv, &stdout, &stderr); status != 0 {
			t.Fatalf("kcfs %s status=%d stderr=%s", command, status, stderr.String())
		}
		if !strings.Contains(stdout.String(), "kcfs plan") || !strings.Contains(stdout.String(), "kcfs mount") {
			t.Fatalf("kcfs %s help omitted supported user commands: %s", command, stdout.String())
		}
	}

	for _, test := range []struct {
		argv []string
		want string
	}{
		{[]string{"unknown"}, "unknown kcfs command"},
		{[]string{"stop"}, "requires one valid --pid"},
		{[]string{"plan"}, "--dataset"},
		{[]string{"mount"}, "--dataset"},
		{[]string{"daemon-mount"}, "missing --dataset"},
		{[]string{"plan", "--pin", "{}", "--dataset", "agent", "--root", "/tmp"}, "rejects --pin"},
		{[]string{"plan", "--workspace", "agent", "--root", "/tmp"}, "flag provided but not defined"},
	} {
		var stdout, stderr bytes.Buffer
		if status := RunKnowledgeSetFS(test.argv, &stdout, &stderr); status == 0 {
			t.Fatalf("kcfs %v unexpectedly succeeded: %s", test.argv, stdout.String())
		}
		if !strings.Contains(stderr.String(), test.want) {
			t.Fatalf("kcfs %v error=%s, want %q", test.argv, stderr.String(), test.want)
		}
	}
}

func TestKnowledgeSetFSRequiresServer(t *testing.T) {
	isolateLoginConfig(t)
	t.Setenv("KC_SERVER_URL", "")
	var stdout, stderr bytes.Buffer
	status := RunKnowledgeSetFS([]string{"plan", "--dataset", "agent", "--root", testkit.TempDir(t)}, &stdout, &stderr)
	if status == 0 || !strings.Contains(stderr.String(), "requires KC Server") {
		t.Fatalf("kcfs bypassed Knowledge Set File Gateway: status=%d stderr=%s", status, stderr.String())
	}
}

func TestResolvedKnowledgeSetPinStripsClientCatalog(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/snapshot/v1/files/mounts:list" {
			t.Errorf("unexpected file gateway request: %s %s", r.Method, r.URL.Path)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["dataset"] != "agent" || request["pin"] != nil {
			t.Errorf("mount must resolve the named Dataset through the gateway: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"pin":{"setId":"agent","revision":1,"pinId":"pin-1","repositories":{"kr://acme/source":"c1"}},"mounts":[{"path":"docs","repository":"kr://acme/source","commit":"c1"}]}`)
	}))
	defer server.Close()
	_, manifest, closeClient, err := prepareKnowledgeSetFS(datasetFSConfig{
		server: server.URL, dataset: "agent", root: t.TempDir(), principal: "agent:test",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeClient()
	stripped, err := json.Marshal(manifest.Pin)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(stripped, &got); err != nil {
		t.Fatal(err)
	}
	if _, found := got["catalog"]; found || got["setId"] != "agent" || got["pinId"] != "pin-1" {
		t.Fatalf("gateway pin = %#v", got)
	}
}

func TestDecodeKnowledgeSetFSReadyDoesNotWaitForEOF(t *testing.T) {
	reader, writer := io.Pipe()
	release := make(chan struct{})
	go func() {
		payload, _ := json.MarshalIndent(datasetFSManifest{
			SetID:    "agent",
			PinID:    "pin-1",
			Root:     "/project",
			ReadOnly: true,
		}, "", "  ")
		_, _ = writer.Write(append(payload, '\n'))
		<-release
		_ = writer.Close()
	}()
	t.Cleanup(func() { close(release) })

	result := make(chan datasetFSManifest, 1)
	errors := make(chan error, 1)
	go func() {
		manifest, err := decodeKnowledgeSetFSReady(reader)
		if err != nil {
			errors <- err
			return
		}
		result <- manifest
	}()

	select {
	case err := <-errors:
		t.Fatal(err)
	case manifest := <-result:
		if manifest.SetID != "agent" || manifest.PinID != "pin-1" || !manifest.ReadOnly {
			t.Fatalf("unexpected ready manifest: %#v", manifest)
		}
	case <-time.After(time.Second):
		t.Fatal("ready decoder waited for mount process EOF")
	}
}
