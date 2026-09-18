package cli_test

import (
	"encoding/json"
	"testing"

	"kc/internal/testkit"
	"kc/knowledge"
)

func readmeSchemaJSON(t *testing.T) string {
	t.Helper()
	for _, operation := range knowledge.SystemSchemaOperations() {
		if operation.Address.ObjectID == knowledge.CoreReadmeSchemaV1 {
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

func TestCatalogShowRepositoriesStayIdentityAndReadmeIsKnowledge(t *testing.T) {
	home := testkit.TempDir(t)
	catalogID := "kr://acme/catalog"
	repo := "kr://acme/payments"
	body(t, kc(home, "init", "--catalog", catalogID))
	seedRepo(t, home, repo)

	catalogs := asMap(t, body(t, kc(home, "catalog", "list")))
	listedCatalogs := catalogs["catalogs"].([]any)
	if len(listedCatalogs) != 1 || asMap(t, listedCatalogs[0])["id"] != catalogID {
		t.Fatalf("catalog list must stay Catalog ids: %#v", catalogs)
	}
	if _, ok := asMap(t, listedCatalogs[0])["repositories"]; ok {
		t.Fatalf("catalog list must not include repository inventory: %#v", listedCatalogs[0])
	}

	before := asMap(t, body(t, kc(home, "show")))
	for _, item := range before["repositories"].([]any) {
		if _, ok := item.(string); ok {
			t.Fatalf("catalog show repositories must be objects: %#v", item)
		}
	}
	listed := inventoryRepository(t, before, repo)
	assertNoInventoryDescription(t, listed)
	system := inventoryRepository(t, before, string(knowledge.SystemRepositoryID))
	assertNoInventoryDescription(t, system)
	if system["schemaCount"] != float64(len(knowledge.SystemSchemaOperations())) {
		t.Fatalf("System Repository schemaCount: %#v", system)
	}

	body(t, kc(home, "writer", "put", "--command-id", "readme-schema", "--repo", repo,
		"--object", string(knowledge.CoreReadmeSchemaV1),
		"--value", readmeSchemaJSON(t)))
	body(t, kc(home, "writer", "put", "--command-id", "readme", "--repo", repo,
		"--object", "payments", "--aspect", knowledge.ReadmeAspect,
		"--schema-ref", string(knowledge.CoreReadmeSchemaV1),
		"--value", `{"body":"# Payments warehouse\n\nPublished metrics and tables for payments."}`))

	after := asMap(t, body(t, kc(home, "show")))
	present := inventoryRepository(t, after, repo)
	assertNoInventoryDescription(t, present)
	if present["schemaCount"] != float64(1) {
		t.Fatalf("schemaCount must count schema/*: %#v", present)
	}
	readme := asMap(t, body(t, kc(home, "read", "--repo", repo, "--object", "payments", "--aspect", knowledge.ReadmeAspect)))
	if asMap(t, readme["value"])["body"] != "# Payments warehouse\n\nPublished metrics and tables for payments." {
		t.Fatalf("README is a knowledge object: %#v", readme)
	}

	body(t, kc(home, "dataset", "define", "--dataset", "payments", "--revision", "1",
		"--source", repo+"=refs/heads/main@knowledge"))
	workspaceView := asMap(t, body(t, kc(home, "show")))
	workspaces := workspaceView["datasets"].([]any)
	if len(workspaces) != 1 || asMap(t, workspaces[0])["id"] != "payments" {
		t.Fatalf("show must list workspace ids: %#v", workspaces)
	}
	workspace := asMap(t, workspaces[0])
	members, _ := workspace["repositories"].([]any)
	if len(members) != 1 || members[0] != repo {
		t.Fatalf("knowledge set members must remain source ids: %#v", workspace)
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
