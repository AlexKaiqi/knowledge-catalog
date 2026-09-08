package integrationruntime_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/client"
	"kc/connector"
	"kc/integrationruntime"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

func TestSchemaRejectedCollectionCanBeRebuiltAndPublishedThroughWriterHTTP(t *testing.T) {
	setup := testkit.NewSetup(t, "kr://kaiqidong/recovery")
	var commands []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Kc-As") != "kaiqidong" {
			t.Error("missing integration identity")
		}
		if request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/head") {
			head, err := setup.Repo.Head(snapshot.DefaultRef)
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"repository": setup.RepositoryID, "commit": head})
			return
		}
		if request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/commits") {
			t.Error("unexpected request")
			w.WriteHeader(404)
			return
		}
		var commit client.CommitRequest
		if err := json.NewDecoder(request.Body).Decode(&commit); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		commands = append(commands, commit.CommandID)
		receipt, err := setup.Writer.Commit(commit.CommandID, commit.ChangeSet)
		if err != nil {
			w.WriteHeader(422)
			_ = json.NewEncoder(w).Encode(kernel.FaultJSON(err))
			return
		}
		_ = json.NewEncoder(w).Encode(receipt)
	}))
	defer server.Close()
	kc, err := client.New(client.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "kaiqidong"}}); err != nil {
		t.Fatal(err)
	}
	runtime, err := integrationruntime.New(t.TempDir(), func(context.Context, string) (integrationruntime.Writer, string, error) {
		return integrationruntime.ClientWriter{Client: kc}, "kaiqidong", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	script := filepath.Join(directory, "collect")
	build := func(priority string) {
		t.Helper()
		output := `{"desired":[{"address":{"kind":"Entity","objectId":"schema/policy/structure/v1"},"value":{"metaSchema":"schema/meta/schema-definition/v1","entity":"Policy","aspect":"structure","pattern":"record","fields":{"priority":{"type":"integer","required":true}}}},{"address":{"kind":"Aspect","objectId":"policy/one","aspectName":"structure"},"schemaRef":"schema/policy/structure/v1","value":{"priority":` + priority + `}}],"sourceRefs":["source://batch/1"],"cursor":"cursor-1"}`
		if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' '"+output+"'\n"), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.Build(context.Background(), integrationruntime.ArtifactSpec{ID: "provider", Directory: directory, Executable: script}); err != nil {
			t.Fatal(err)
		}
	}
	manifest := integrationruntime.Manifest{ID: "sync", ArtifactID: "provider", Server: server.URL, Repository: setup.RepositoryID, Mode: connector.ModePatch, Scope: connector.Scope{AllowEntity: true, Aspects: []string{"structure"}}, IntervalSeconds: 60}
	build(`"high"`)
	if _, err := runtime.Activate(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Run(context.Background(), manifest.ID); kernel.CodeOf(err) != kernel.ErrSchemaInstanceInvalid {
		t.Fatalf("wanted actual schema rejection, got %v", err)
	}
	status, err := runtime.Status(manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Checkpoint.Cursor != "" || status.Checkpoint.Commit != "" {
		t.Fatal("rejected collection advanced checkpoint")
	}
	if head := testkit.MustHead(t, setup.Repo, snapshot.DefaultRef); head != setup.RootCommitID {
		t.Fatal("rejected collection advanced Writer target")
	}
	build("7")
	if _, err := runtime.Activate(context.Background(), manifest); err != nil {
		t.Fatalf("corrected artifact cannot replace a definitively rejected collection: %v", err)
	}
	status, err = runtime.Run(context.Background(), manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != "PUBLISHED" || status.Checkpoint.Cursor != "cursor-1" || status.PendingCommandID != "" {
		t.Fatalf("corrected source did not publish: %#v", status)
	}
	if len(commands) != 2 || commands[0] == commands[1] {
		t.Fatal("corrected payload did not get its own Writer command")
	}
	if got := testkit.MustHead(t, setup.Repo, snapshot.DefaultRef); got != status.Checkpoint.Commit {
		t.Fatal("checkpoint differs from acknowledged Writer publication")
	}
}
