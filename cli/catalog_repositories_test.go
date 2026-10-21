package cli_test

import (
	"encoding/json"
	"kc/systemrepo"
	"testing"

	"kc/internal/testkit"
	objectmodel "kc/object"
)

func readmeSchemaJSON(t *testing.T) string {
	t.Helper()
	for _, operation := range systemrepo.SystemSchemaOperations() {
		if operation.Address.ObjectID == objectmodel.ObjectID("schema/core/readme/v1") {
			raw, err := json.Marshal(operation.Value)
			if err != nil {
				t.Fatal(err)
			}
			return string(raw)
		}
	}
	t.Fatal("readme schema is not published")
	return ""
}

func TestCatalogShowRepositoriesStayIdentityAndReadmeIsObject(t *testing.T) {
	home := testkit.TempDir(t)
	repo := "kr://acme/payments"
	body(t, kc(home, "init"))
	seedRepo(t, home, repo)

	retired := kc(home, "catalog", "list")
	expectCode(t, retired, "USAGE_INVALID")
	before := asMap(t, body(t, kc(home, "show")))
	for _, item := range before["repositories"].([]any) {
		if _, ok := item.(string); ok {
			t.Fatalf("catalog show repositories must be objects: %#v", item)
		}
	}
	listed := inventoryRepository(t, before, repo)
	assertNoInventoryDescription(t, listed)
	system := inventoryRepository(t, before, string(objectmodel.SystemRepositoryID))
	assertNoInventoryDescription(t, system)
	if system["schemaCount"] != float64(len(systemrepo.SystemSchemaOperations())) {
		t.Fatalf("System Repository schemaCount: %#v", system)
	}

	body(t, kc(home, "writer", "put", testObjectReference(t, repo, string(objectmodel.ObjectID("schema/core/readme/v1")), ""), "--repo", repo, "--command-id", "readme-schema", "--value", readmeSchemaJSON(t)))
	body(t, kc(home, "writer", "put", testObjectReference(t, repo, "fixture/payments", testObjectLocator("readme")), "--repo", repo, "--command-id", "readme",
		"--schema-ref", string(objectmodel.ObjectID("schema/core/readme/v1")),
		"--value", `{"body":"# Payments warehouse\n\nPublished metrics and tables for payments."}`))

	after := asMap(t, body(t, kc(home, "show")))
	present := inventoryRepository(t, after, repo)
	assertNoInventoryDescription(t, present)
	if present["schemaCount"] != float64(1) {
		t.Fatalf("schemaCount must count schema/*: %#v", present)
	}
	readme := asMap(t, body(t, kc(home, "read", testObjectReference(t, repo, "fixture/payments", testObjectLocator("readme")), "--repo", repo)))
	if asMap(t, readme["value"])["body"] != "# Payments warehouse\n\nPublished metrics and tables for payments." {
		t.Fatalf("README is an object: %#v", readme)
	}

	body(t, kc(home, "dataset", "define", "--dataset", "payments", "--revision", "1",
		"--source", repo+"=refs/heads/main@object"))
	workspaceView := asMap(t, body(t, kc(home, "show")))
	workspaces := workspaceView["datasets"].([]any)
	if len(workspaces) != 1 || asMap(t, workspaces[0])["id"] != "payments" {
		t.Fatalf("show must list workspace ids: %#v", workspaces)
	}
	workspace := asMap(t, workspaces[0])
	members, _ := workspace["repositories"].([]any)
	if len(members) != 1 || members[0] != repo {
		t.Fatalf("dataset members must remain source ids: %#v", workspace)
	}
}

func assertNoInventoryDescription(t *testing.T, row map[string]any) {
	t.Helper()
	if _, ok := row["title"]; ok {
		t.Fatalf("catalog inventory must not flatten README into title: %#v", row)
	}
	if _, ok := row["summary"]; ok {
		t.Fatalf("catalog inventory must not flatten README into summary: %#v", row)
	}
}

func inventoryRepository(t *testing.T, payload map[string]any, repoID string) map[string]any {
	t.Helper()
	raw, _ := payload["repositories"].([]any)
	for _, item := range raw {
		row := asMap(t, item)
		if row["id"] == repoID {
			return row
		}
	}
	t.Fatalf("missing repository %s in %#v", repoID, payload["repositories"])
	return nil
}
