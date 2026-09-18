package cli_test

import (
	"net/http/httptest"
	"testing"

	"kc/cli"
	apphome "kc/home"
	"kc/knowledge"
)

func TestAuthenticatedPrincipalDiscoversPublicCatalogWithoutGrant(t *testing.T) {
	cfg, path := declaredDeployment(t, true)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer func() {
		server.Close()
		if closer, ok := handler.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()
	catalogID := cfg.Catalogs[0].ID
	listed := asMap(t, body(t, kcRemote(t, server.URL, "agent:viewer", "catalog", "list")))["catalogs"].([]any)
	if len(listed) != 1 || asMap(t, listed[0])["id"] != catalogID {
		t.Fatalf("public Catalog missing from inventory: %#v", listed)
	}
	show := remoteCatalogShow(t, server.URL, "agent:viewer", catalogID)
	if show["catalogId"] != catalogID {
		t.Fatalf("public Catalog hidden from authenticated viewer: %#v", show)
	}
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "read", "--repo", cfg.Repositories[0].ID, "--object", "note/one"), "FORBIDDEN")
}

func TestPrivateCatalogStillRequiresCatalogReadGrant(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	cfg.Catalogs[0].Private = true
	writeDeployment(t, path, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer func() {
		server.Close()
		if closer, ok := handler.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "catalog", "list"), "FORBIDDEN")
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "catalog", "use", cfg.Catalogs[0].ID), "FORBIDDEN")
	body(t, kcRemote(t, server.URL, cfg.BootstrapPrincipal, "grant", "add", "--principal", "agent:viewer", "--action", "catalog.read", "--catalog", cfg.Catalogs[0].ID))
	show := remoteCatalogShow(t, server.URL, "agent:viewer", cfg.Catalogs[0].ID)
	if show["catalogId"] != cfg.Catalogs[0].ID {
		t.Fatalf("granted private Catalog still hidden: %#v", show)
	}
}

func TestUndeclaredSystemRepositoryStillRequiresGrant(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	cfg.RepositoryAccess = nil
	writeDeployment(t, path, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer func() {
		server.Close()
		if closer, ok := handler.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "schema", "list", "--repo", string(knowledge.SystemRepositoryID)), "FORBIDDEN")
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "read", "--repo", string(knowledge.SystemRepositoryID), "--object", string(knowledge.MetaSchemaV1)), "FORBIDDEN")
}

func TestAuthenticatedPrincipalReadsDeclaredRepositoryWithoutGrant(t *testing.T) {
	cfg, path := declaredDeployment(t, true)
	cfg.RepositoryAccess = []apphome.RepositoryAccess{{
		ID:                   cfg.Repositories[0].ID,
		AuthenticatedActions: []string{"knowledge.read", "knowledge.schema.read"},
	}}
	writeDeployment(t, path, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer func() {
		server.Close()
		if closer, ok := handler.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()
	listed := asMap(t, body(t, kcRemote(t, server.URL, "agent:viewer", "schema", "list", "--repo", cfg.Repositories[0].ID)))
	if listed["repository"] != cfg.Repositories[0].ID {
		t.Fatalf("declared authenticated read must admit schema browse: %#v", listed)
	}
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "read", "--repo", cfg.Repositories[0].ID, "--object", "note/one"), "KNOWLEDGE_REF_UNRESOLVED")
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "schema", "list", "--repo", string(knowledge.SystemRepositoryID)), "FORBIDDEN")
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "writer", "put", "--command-id", "mutate-declared", "--repo", cfg.Repositories[0].ID, "--object", "note/one", "--value", `{"v":1}`), "FORBIDDEN")
}

func TestDeclaredSystemRepositoryUsesAuthenticatedDefault(t *testing.T) {
	_, path := declaredDeployment(t, false)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer func() {
		server.Close()
		if closer, ok := handler.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()
	report := asMap(t, body(t, kcRemote(t, server.URL, "agent:viewer", "schema", "list", "--repo", string(knowledge.SystemRepositoryID))))
	if report["repository"] != string(knowledge.SystemRepositoryID) {
		t.Fatalf("declared System Repository must be readable like any other authenticated-default repo: %#v", report)
	}
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "writer", "put", "--command-id", "mutate-system", "--repo", string(knowledge.SystemRepositoryID), "--object", "schema/evil", "--value", `{"entity":"Evil"}`), "FORBIDDEN")
}
