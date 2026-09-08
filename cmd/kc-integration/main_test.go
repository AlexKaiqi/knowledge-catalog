package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/cli"
	"kc/client"
)

func TestCommandBuildActivateRunReusesKCLogin(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_SERVER_URL", "")
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_AS", "")
	commits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity/v1/auth" {
			_, _ = w.Write([]byte(`{"mode":"gitea","localAssertion":false}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer saved-user-token" || r.Header.Get("X-Kc-As") != "" {
			t.Error("integration command failed to reuse credential-only KC login")
		}
		if r.URL.Path == "/identity/v1/whoami" {
			_, _ = w.Write([]byte(`{"principal":"kaiqidong"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/head") {
			_, _ = w.Write([]byte(`{"repository":"kr://kaiqidong/notes","commit":"base"}`))
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commits") {
			commits++
			var request client.CommitRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.ChangeSet.TargetRepository != "kr://kaiqidong/notes" || request.ChangeSet.BaseCommit != "base" {
				t.Error("collector changed the fixed Writer target")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"commandId": request.CommandID, "result": map[string]string{"repositoryId": "kr://kaiqidong/notes", "commitId": "published"}})
			return
		}
		t.Errorf("unexpected API %s %s", r.Method, r.URL.Path)
		w.WriteHeader(404)
	}))
	defer server.Close()
	if result := cli.Run([]string{"login", "--server", server.URL, "--mode", "token", "--token", "saved-user-token"}); result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	directory := t.TempDir()
	script := filepath.Join(directory, "collect")
	stateDir := t.TempDir()
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' '{\"desired\":[{\"address\":{\"kind\":\"Entity\",\"objectId\":\"note/one\"},\"value\":{\"body\":\"from runtime\"}}],\"sourceRefs\":[\"source://batch/1\"],\"cursor\":\"1\"}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	artifactFile := filepath.Join(directory, "artifact.json")
	manifestFile := filepath.Join(directory, "manifest.json")
	artifact, _ := json.Marshal(map[string]any{"id": "collector", "directory": directory, "executable": script})
	if err := os.WriteFile(artifactFile, artifact, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestFile, []byte(`{"id":"notes","artifactId":"collector","repository":"kr://kaiqidong/notes","mode":"patch","scope":{"allowEntity":true},"intervalSeconds":60}`), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	for _, args := range [][]string{{"build", "--file", artifactFile}, {"activate", "--file", manifestFile}, {"run", "--id", "notes"}, {"status", "--id", "notes"}, {"pause", "--id", "notes"}} {
		output.Reset()
		args = append(args, "--state-dir", stateDir)
		if err := run(context.Background(), args, &output); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output.String(), "saved-user-token") {
			t.Fatal("command output disclosed KC token")
		}
	}
	if commits != 1 {
		t.Fatalf("publication calls = %d", commits)
	}
}
