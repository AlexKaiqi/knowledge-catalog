package cli_test

import (
	"context"
	kcclient "kc/client"
	apphome "kc/home"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The fixture is an already deployed platform. Its configuration never names
// the user's target repository; every provider operation crosses Run -> HTTP.
func TestManagedRepositoryProviderCreatesPublishesAndResumes(t *testing.T) {
	cfg, configPath := declaredDeployment(t, false)
	catalogID, repositoryID := cfg.Registry.ID, "kr://scene/managed-object"
	provider, ungranted := "provider", "ungranted"
	lakefsFake := testkit.NewLakeFSFake(t)
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	cfg.ManagedRepositories = &apphome.ManagedRepositoryConfig{
		Driver: "lakefs", DSN: lakefsFake.Origin(), Root: "s3://kc-authority",
		CreatorActions: []string{"writer.preview", "writer.commit", "object.read", "object.provenance"},
	}
	writeDeployment(t, configPath, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", configPath))
	var server *httptest.Server
	var handler http.Handler
	start := func() {
		var err error
		handler, err = serv.HTTPHandlerFromConfig(configPath, serv.HTTPServerOptions{})
		if err != nil {
			t.Fatal(err)
		}
		server = httptest.NewServer(handler)
	}
	stop := func() {
		server.Close()
		if err := handler.(interface{ Close() error }).Close(); err != nil {
			t.Fatal(err)
		}
		server = nil
	}
	start()
	t.Cleanup(func() {
		if server != nil {
			stop()
		}
	})
	govern := func(args ...string) kcRunResult { return kcRemote(t, server.URL, cfg.BootstrapPrincipal, args...) }
	provide := func(args ...string) kcRunResult { return kcRemote(t, server.URL, provider, args...) }
	createProtocolRepository := func(principal, repository, commandID string) (map[string]any, error) {
		typed, err := kcclient.New(kcclient.Config{BaseURL: server.URL, HTTPClient: server.Client()})
		if err != nil {
			return nil, err
		}
		if _, err := typed.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: principal}}); err != nil {
			return nil, err
		}
		var out map[string]any
		err = typed.RepositoryManagementService().CreateRepository(context.Background(), kcclient.RepositoryCreateRequest{
			Repository: repository, CommandID: commandID,
		}, kcclient.RequestOptions{}, &out)
		return out, err
	}
	// Existing admission policy permits creation, but does not pre-grant any
	// target repository action or give this user catalog/admin management.
	body(t, govern("grant", "add", "--principal", provider, "--action", "repositories.create,repositories.manage"))
	if shown := asMap(t, body(t, provide("show"))); shown["catalogId"] != nil {
		t.Fatalf("authenticated provider must discover the public Catalog: %#v", shown)
	}
	expectCode(t, provide("grant", "list"), "FORBIDDEN")
	before := remoteCatalogShow(t, server.URL, cfg.BootstrapPrincipal, catalogID)
	if _, err := createProtocolRepository(ungranted, repositoryID, "ungranted-create"); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("ungranted create: %v", err)
	}
	if after := remoteCatalogShow(t, server.URL, cfg.BootstrapPrincipal, catalogID); !reflect.DeepEqual(after, before) {
		t.Fatal("denied create changed Catalog membership")
	}
	created, err := createProtocolRepository(provider, repositoryID, "provider-create")
	if err != nil {
		t.Fatal(err)
	}
	if created["status"] != "APPLIED" || created["catalog"] != nil || created["repositoryId"] != repositoryID || created["commandId"] != "provider-create" || created["head"] == "" || created["head"] == nil {
		t.Fatalf("create did not return a ready repository: %#v", created)
	}
	body(t, kcRemote(t, server.URL, provider, "attach", "--repo", repositoryID))
	if inventory := remoteCatalogShow(t, server.URL, cfg.BootstrapPrincipal, catalogID); !hasRepository(inventory, repositoryID) {
		t.Fatalf("attached repository was not admitted: %#v", inventory)
	}
	// A repeated click with another command cannot allocate the same logical
	// repository again; changing a persisted command's target cannot create a
	// second source. Both are asserted through the real Client/HTTP boundary.
	admitted := remoteCatalogShow(t, server.URL, cfg.BootstrapPrincipal, catalogID)
	headBefore := body(t, provide("writer", "head", "--repo", repositoryID))
	if _, err := createProtocolRepository(provider, repositoryID, "provider-competing-create"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("competing create: %v", err)
	}
	if _, err := createProtocolRepository(provider, "kr://scene/another-target", "provider-create"); kernel.CodeOf(err) != kernel.ErrIdempotencyConflict {
		t.Fatalf("changed create: %v", err)
	}
	if after := remoteCatalogShow(t, server.URL, cfg.BootstrapPrincipal, catalogID); !reflect.DeepEqual(after, admitted) {
		t.Fatal("conflicting creation changed Catalog membership")
	}
	if after := body(t, provide("writer", "head", "--repo", repositoryID)); !reflect.DeepEqual(after, headBefore) {
		t.Fatal("conflicting creation changed the original source")
	}
	putArgs := []string{"writer", "put", testObjectReference(t, repositoryID, "note/managed", ""), "--repo", repositoryID, "--command-id", "provider-first", "--if-absent", "--value", `{"text":"first publication"}`, "--origin-kind", "SOURCE", "--source-ref", "file:///scene/source/managed.md"}
	firstReceipt := asMap(t, body(t, provide(putArgs...)))
	first := publishedCommit(t, firstReceipt)
	if firstReceipt["disposition"] != "APPLIED" || first == created["head"] {
		t.Fatalf("first publication did not advance the new repository: %#v", firstReceipt)
	}
	assertRead := func(object, commit, text string) map[string]any {
		t.Helper()
		row := asMap(t, body(t, provide("read", testObjectReference(t, repositoryID, object, ""), "--repo", repositoryID, "--commit", commit)))
		if row["commit"] != commit || asMap(t, row["value"])["text"] != text {
			t.Fatalf("read did not preserve published version/content: %#v", row)
		}
		return row
	}
	assertRead("note/managed", first, "first publication")
	provenance := asMap(t, body(t, provide("provenance", "--repo", repositoryID, "--object", "note/managed", "--commit", first)))
	if provenance["commit"] != first {
		t.Fatalf("provenance lost the publication basis: %#v", provenance)
	}
	chain := provenance["chain"].([]any)
	if len(chain) == 0 || !reflect.DeepEqual(asMap(t, chain[0])["sourceRefs"], []any{"file:///scene/source/managed.md"}) {
		t.Fatalf("provenance lost the provider's source: %#v", provenance)
	}

	draftDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(draftDir, "batch.json"), []byte("---\nobject_id: note/batch\n---\n{\"text\":\"batch publication\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	batch := publishedCommit(t, asMap(t, body(t, provide("writer", "commit", "--command-id", "provider-batch", "--repo", repositoryID, "--dir", draftDir))))
	assertRead("note/batch", batch, "batch publication")
	expectCode(t, kcRemote(t, server.URL, ungranted, "read", testObjectReference(t, repositoryID, "note/managed", ""), "--repo", repositoryID, "--commit", first), "FORBIDDEN")
	expectCode(t, kcRemote(t, server.URL, ungranted, "writer", "put", testObjectReference(t, repositoryID, "note/managed", ""), "--repo", repositoryID, "--command-id", "ungranted-write", "--value", `{}`), "FORBIDDEN")

	// Instance replacement is a platform event between two provider requests.
	// It may discard caches, but never add the dynamic target to static config.
	stop()
	if err := os.RemoveAll(cfg.CacheDir); err != nil {
		t.Fatal(err)
	}
	cfg.CacheDir = filepath.Join(filepath.Dir(cfg.CacheDir), "managed-replacement-instance")
	if len(cfg.Repositories) != 1 || cfg.Repositories[0].ID != string(objectmodel.SystemRepositoryID) {
		t.Fatalf("static deployment bindings must contain only the required system authority: %#v", cfg.Repositories)
	}
	writeDeployment(t, configPath, cfg)
	start()
	recreated, err := createProtocolRepository(provider, repositoryID, "provider-create")
	if err != nil {
		t.Fatal(err)
	}
	if recreated["status"] != "REPLAYED" || recreated["head"] != created["head"] || recreated["repositoryId"] != repositoryID {
		t.Fatalf("restart did not preserve creation result: %#v", recreated)
	}
	replayed := asMap(t, body(t, provide(putArgs...)))
	if replayed["disposition"] != "REPLAYED" || publishedCommit(t, replayed) != first {
		t.Fatalf("restart lost the Writer idempotency record: %#v", replayed)
	}
	assertRead("note/managed", first, "first publication")
	assertRead("note/managed", batch, "first publication")
	current := asMap(t, body(t, provide("resolve", "--repo", repositoryID, "--object", "note/managed", "--commit", batch)))
	digest, ok := current["digest"].(string)
	if !ok || digest == "" {
		t.Fatalf("read did not expose the update precondition: %#v", current)
	}
	updated := publishedCommit(t, asMap(t, body(t, provide("writer", "put", testObjectReference(t, repositoryID, "note/managed", ""), "--repo", repositoryID, "--command-id", "provider-update", "--expected", batch, "--if-digest", digest, "--value", `{"text":"after replacement"}`, "--origin-kind", "SOURCE", "--source-ref", "file:///scene/source/managed.md"))))
	assertRead("note/managed", updated, "after replacement")
	assertRead("note/managed", first, "first publication")

	// Revocation is a separate management event, not a provider task step.
	// Creation replay and another restart must never silently re-grant access.
	rules := asMap(t, body(t, govern("grant", "list")))["rules"].([]any)
	revoked := 0
	for _, raw := range rules {
		rule := asMap(t, raw)
		if rule["principal"] == provider && rule["repo"] == repositoryID {
			body(t, govern("grant", "remove", "--id", rule["id"].(string)))
			revoked++
		}
	}
	if revoked == 0 {
		t.Fatal("creation did not establish real, revocable target grants")
	}
	stop()
	start()
	replayedCreate, err := createProtocolRepository(provider, repositoryID, "provider-create")
	if err != nil {
		t.Fatal(err)
	}
	if replayedCreate["status"] != "REPLAYED" {
		t.Fatalf("revocation changed creation history: %#v", replayedCreate)
	}
	expectCode(t, provide("read", testObjectReference(t, repositoryID, "note/managed", ""), "--repo", repositoryID, "--commit", first), "FORBIDDEN")
	expectCode(t, provide("writer", "put", testObjectReference(t, repositoryID, "note/revoked", ""), "--repo", repositoryID, "--command-id", "revoked-write", "--value", `{}`), "FORBIDDEN")
}
