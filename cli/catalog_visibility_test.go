package cli_test

import (
	"fmt"
	serv "kc/server"
	"net/http/httptest"
	"strings"
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

func TestAuthenticatedPrincipalConsumesDeclaredDatasetWithoutGrant(t *testing.T) {
	cfg, path := declaredDeployment(t, true)
	cfg.DatasetAccess = []apphome.DatasetAccess{{ID: "public-kit", AuthenticatedActions: apphome.AuthenticatedDatasetActions()}}
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
	call := func(args ...string) kcRunResult { return kcRemote(t, server.URL, cfg.BootstrapPrincipal, args...) }
	body(t, call("attach", "--repo", "kr://recover/source"))
	materials := t.TempDir()
	writeSourceTree(t, materials, "note/public.md", "public\n")
	body(t, call("writer", "commit", "--command-id", "public-write", "--repo", "kr://recover/source", "--dir", materials))
	body(t, call("dataset", "define", "public-kit", "--revision", "1", "--source", "kr://recover/source@notes@note"))
	body(t, call("dataset", "define", "private-kit", "--revision", "1", "--source", "kr://recover/source@notes@note"))

	viewer := func(args ...string) kcRunResult { return kcRemote(t, server.URL, "agent:viewer", args...) }
	if shown := fmt.Sprint(body(t, viewer("show"))); !strings.Contains(shown, "public-kit") {
		t.Fatalf("declared Dataset must be discoverable in the inventory: %s", shown)
	}
	read := asMap(t, body(t, viewer("files", "read", "--dataset", "public-kit", "--revision", "1", "--path", "notes/public.md")))
	if read["revision"] == nil {
		t.Fatalf("declared Dataset must deliver bytes to any authenticated principal: %#v", read)
	}
	body(t, viewer("files", "list", "--dataset", "public-kit", "--path", "", "--limit", "10"))
	expectCode(t, viewer("files", "read", "--dataset", "private-kit", "--revision", "1", "--path", "notes/public.md"), "FORBIDDEN")
	expectCode(t, viewer("read", testObjectReference(t, "kr://recover/source", "note/public", ""), "--repo", "kr://recover/source"), "FORBIDDEN")
	expectCode(t, viewer("dataset", "define", "public-kit", "--revision", "2", "--source", "kr://recover/source@notes@note"), "FORBIDDEN")
	expectCode(t, viewer("writer", "put", testObjectReference(t, "kr://recover/source", "note/evil", ""), "--repo", "kr://recover/source", "--command-id", "public-mutate", "--value", `{"v":1}`), "FORBIDDEN")
	grants := asMap(t, body(t, call("grant", "list")))
	for _, rule := range grants["rules"].([]any) {
		if id, _ := rule.(map[string]any)["id"].(string); strings.HasPrefix(id, "dataset-access:") {
			t.Fatalf("declared Dataset access must not materialize as a grant: %#v", rule)
		}
	}
}

func TestDatasetAccessRejectsActionsOutsideConsumption(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	for _, items := range [][]apphome.DatasetAccess{
		{{ID: "kit", AuthenticatedActions: []string{"dataset.define"}}},
		{{ID: "kit", AuthenticatedActions: []string{"file.read"}}, {ID: "kit", AuthenticatedActions: []string{"file.read"}}},
		{{ID: "", AuthenticatedActions: []string{"file.read"}}},
	} {
		cfg.DatasetAccess = items
		writeDeployment(t, path, cfg)
		expectCode(t, deploymentCommand(t, "deployment", "init", "--config", path), "USAGE_INVALID")
	}
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
