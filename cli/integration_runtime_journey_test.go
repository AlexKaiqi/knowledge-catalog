package cli_test

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/cli"
	"kc/client"
	"kc/connector"
	"kc/integrationruntime"
	"kc/internal/testkit"
	"kc/kernel"
)

func TestIntegrationRuntimePublishesThroughAuthenticatedWriterHTTP(t *testing.T) {
	home := testkit.TempDir(t)
	const catalogID = "kr://runtime/catalog"
	const repository = "kr://runtime/source"
	const principal = "kaiqidong"
	body(t, kc(home, "local", "init", "--catalog", catalogID))
	seedRepo(t, home, repository)
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", principal))
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	isolateClientCredentials(t)
	t.Setenv("KC_SERVER_URL", "")
	login := cli.Run([]string{"login", "--server", server.URL, "--mode", "local", "--as", principal})
	if login.Status != 0 {
		t.Fatal(login.Stdout)
	}
	factory := func(ctx context.Context, server string) (integrationruntime.Writer, string, error) {
		kc, err := cli.NewSessionClient(ctx, server, "")
		if err != nil {
			return nil, "", err
		}
		identity, err := kc.IdentityService().WhoAmI(ctx, client.RequestOptions{})
		return integrationruntime.ClientWriter{Client: kc}, identity.Principal, err
	}
	runtime, err := integrationruntime.New(t.TempDir(), factory)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	script := filepath.Join(directory, "collect")
	content := `#!/bin/sh
printf '%s\n' '{"desired":[{"address":{"kind":"Entity","objectId":"note/published"},"value":{"text":"published through runtime"}}],"observed":[],"sourceRefs":["source://integration/batch-1"],"cursor":"batch-1"}'
`
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Build(context.Background(), integrationruntime.ArtifactSpec{ID: "provider", Directory: directory, Executable: script}); err != nil {
		t.Fatal(err)
	}
	manifest := integrationruntime.Manifest{ID: "provider-sync", ArtifactID: "provider", Server: cli.ClientServerURL(""), Repository: kernel.RepositoryID(repository), Mode: connector.ModePatch, Scope: connector.Scope{AllowEntity: true, ObjectPrefix: "note/"}, IntervalSeconds: 60}
	if _, err := runtime.Activate(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	status, err := runtime.Run(context.Background(), manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Owner != principal || status.Checkpoint.Cursor != "batch-1" || status.Checkpoint.Commit == "" || status.Phase != "PUBLISHED" {
		t.Fatalf("publication not acknowledged: %#v", status)
	}
	read := cli.Run([]string{"knowledge", "read", "--repo", repository, "--object", "note/published", "--commit", string(status.Checkpoint.Commit)})
	if read.Status != 0 {
		t.Fatal(read.Stdout)
	}
	if !strings.Contains(read.Stdout, "published through runtime") {
		t.Fatalf("fixed-version read did not return the collected value: %s", read.Stdout)
	}
	if status.PendingCommandID != "" {
		t.Fatal("completed Writer operation retained a pending request")
	}
}
