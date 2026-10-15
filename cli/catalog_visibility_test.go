package cli_test

import (
	serv "kc/server"
	"net/http/httptest"
	"testing"

	apphome "kc/home"
	objectmodel "kc/object"
)

func TestAuthenticatedPrincipalDiscoversPublicCatalogWithoutGrant(t *testing.T) {
	cfg, path := declaredDeployment(t, true)
	source, err := cfg.Binding("kr://recover/source")
	if err != nil {
		t.Fatal(err)
	}
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := serv.HTTPHandlerFromConfig(path, serv.HTTPServerOptions{})
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
	catalogID := cfg.Registry.ID
	show := remoteCatalogShow(t, server.URL, "agent:viewer", catalogID)
	if _, present := show["catalogId"]; present || len(show["repositories"].([]any)) == 0 {
		t.Fatalf("service inventory shape changed: %#v", show)
	}
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "read", testObjectReference(t, source.ID, "note/one", ""), "--repo", source.ID), "FORBIDDEN")
}

func TestRegistryHistoryRequiresExplicitAuditReadGrant(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	writeDeployment(t, path, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := serv.HTTPHandlerFromConfig(path, serv.HTTPServerOptions{})
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
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "registry", "audit"), "FORBIDDEN")
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "catalog", "use", "old"), "USAGE_INVALID")
	body(t, kcRemote(t, server.URL, cfg.BootstrapPrincipal, "grant", "add", "--principal", "agent:viewer", "--action", "registry.audit.read"))
	show := remoteCatalogShow(t, server.URL, "agent:viewer", cfg.Registry.ID)
	if _, present := show["catalogId"]; present || len(show["repositories"].([]any)) == 0 {
		t.Fatalf("granted private Catalog still hidden: %#v", show)
	}
}

func TestUndeclaredSystemRepositoryStillRequiresGrant(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	cfg.RepositoryAccess = nil
	writeDeployment(t, path, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := serv.HTTPHandlerFromConfig(path, serv.HTTPServerOptions{})
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
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "schema", "list", "--repo", string(objectmodel.SystemRepositoryID)), "FORBIDDEN")
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "read", testObjectReference(t, string(objectmodel.SystemRepositoryID), string(objectmodel.MetaSchemaV1), ""), "--repo", string(objectmodel.SystemRepositoryID)), "FORBIDDEN")
}

func TestAuthenticatedPrincipalReadsDeclaredRepositoryWithoutGrant(t *testing.T) {
	cfg, path := declaredDeployment(t, true)
	source, err := cfg.Binding("kr://recover/source")
	if err != nil {
		t.Fatal(err)
	}
	cfg.RepositoryAccess = []apphome.RepositoryAccess{{
		ID:                   source.ID,
		AuthenticatedActions: []string{"object.read", "object.schema.read"},
	}}
	writeDeployment(t, path, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := serv.HTTPHandlerFromConfig(path, serv.HTTPServerOptions{})
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
	listed := asMap(t, body(t, kcRemote(t, server.URL, "agent:viewer", "schema", "list", "--repo", source.ID)))
	if listed["repository"] != source.ID {
		t.Fatalf("declared authenticated read must admit schema browse: %#v", listed)
	}
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "read", testObjectReference(t, source.ID, "note/one", ""), "--repo", source.ID), "OBJECT_REF_UNRESOLVED")
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "schema", "list", "--repo", string(objectmodel.SystemRepositoryID)), "FORBIDDEN")
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "writer", "put", testObjectReference(t, source.ID, "note/one", ""), "--repo", source.ID, "--command-id", "mutate-declared", "--value", `{"v":1}`), "FORBIDDEN")
}

func TestDeclaredSystemRepositoryUsesAuthenticatedDefault(t *testing.T) {
	_, path := declaredDeployment(t, false)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := serv.HTTPHandlerFromConfig(path, serv.HTTPServerOptions{})
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
	report := asMap(t, body(t, kcRemote(t, server.URL, "agent:viewer", "schema", "list", "--repo", string(objectmodel.SystemRepositoryID))))
	if report["repository"] != string(objectmodel.SystemRepositoryID) {
		t.Fatalf("declared System Repository must be readable like any other authenticated-default repo: %#v", report)
	}
	expectCode(t, kcRemote(t, server.URL, "agent:viewer", "writer", "put", testObjectReference(t, string(objectmodel.SystemRepositoryID), "schema/evil", ""), "--repo", string(objectmodel.SystemRepositoryID), "--command-id", "mutate-system", "--value", `{"entity":"Evil"}`), "FORBIDDEN")
}
