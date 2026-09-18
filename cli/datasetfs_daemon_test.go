package cli

import (
	"bytes"
	"encoding/json"
	"io"
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
	raw := json.RawMessage(`{"catalog":"kr://acme/catalog","setId":"agent","revision":1,"pinId":"pin-1","repositories":{"kr://acme/source":"c1"}}`)
	stripped, err := resolvedKnowledgeSetPin(raw)
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
			SetID: "agent",
			PinID:       "pin-1",
			Root:        "/project",
			ReadOnly:    true,
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
