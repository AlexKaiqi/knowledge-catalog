package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceFSPublicCommandAndUsageSurface(t *testing.T) {
	for _, command := range []string{"", "help", "--help", "-h"} {
		var stdout, stderr bytes.Buffer
		argv := []string{}
		if command != "" {
			argv = []string{command}
		}
		if status := RunWorkspaceFS(argv, &stdout, &stderr); status != 0 {
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
		{[]string{"plan"}, "--workspace"},
		{[]string{"mount"}, "--workspace"},
		{[]string{"daemon-mount"}, "missing --workspace"},
	} {
		var stdout, stderr bytes.Buffer
		if status := RunWorkspaceFS(test.argv, &stdout, &stderr); status == 0 {
			t.Fatalf("kcfs %v unexpectedly succeeded: %s", test.argv, stdout.String())
		}
		if !strings.Contains(stderr.String(), test.want) {
			t.Fatalf("kcfs %v error=%s, want %q", test.argv, stderr.String(), test.want)
		}
	}
}

func TestDecodeWorkspaceFSReadyDoesNotWaitForEOF(t *testing.T) {
	reader, writer := io.Pipe()
	release := make(chan struct{})
	go func() {
		payload, _ := json.MarshalIndent(workspaceFSManifest{
			WorkspaceID: "agent",
			PinID:       "pin-1",
			Root:        "/project",
			ReadOnly:    true,
		}, "", "  ")
		_, _ = writer.Write(append(payload, '\n'))
		<-release
		_ = writer.Close()
	}()
	t.Cleanup(func() { close(release) })

	result := make(chan workspaceFSManifest, 1)
	errors := make(chan error, 1)
	go func() {
		manifest, err := decodeWorkspaceFSReady(reader)
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
		if manifest.WorkspaceID != "agent" || manifest.PinID != "pin-1" || !manifest.ReadOnly {
			t.Fatalf("unexpected ready manifest: %#v", manifest)
		}
	case <-time.After(time.Second):
		t.Fatal("ready decoder waited for mount process EOF")
	}
}
