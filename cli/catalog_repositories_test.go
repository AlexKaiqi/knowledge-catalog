package cli_test

import (
	"encoding/json"
	"testing"

	"kc/internal/testkit"
	"kc/knowledge"
)

func sourceProfileSchemaJSON(t *testing.T) string {
	t.Helper()
	for _, operation := range knowledge.SystemSchemaOperations() {
		if operation.Address.ObjectID == knowledge.CoreSourceProfileSchemaV1 {
			raw, err := json.Marshal(operation.Value)
			if err != nil {
				t.Fatal(err)
			}
			return string(raw)
		}
	}
	t.Fatal("source profile schema is not published")
	return ""
}

func TestCatalogShowRepositoriesIncludeSourceProfile(t *testing.T) {
	home := testkit.TempDir(t)
	catalogID := "kr://acme/catalog"
	repo := "kr://acme/payments"
	body(t, kc(home, "init", "--catalog", catalogID))
	body(t, kc(home, "repo-add", "--repo", repo))

	catalogs := asMap(t, body(t, kc(home, "catalog", "list")))
	listedCatalogs := catalogs["catalogs"].([]any)
	if len(listedCatalogs) != 1 || asMap(t, listedCatalogs[0])["id"] != catalogID {
		t.Fatalf("catalog list must stay Catalog ids: %#v", catalogs)
	}
	if _, ok := asMap(t, listedCatalogs[0])["repositories"]; ok {
		t.Fatalf("catalog list must not include repository inventory: %#v", listedCatalogs[0])
	}

	before := asMap(t, body(t, kc(home, "catalog", "show")))
	for _, item := range before["repositories"].([]any) {
		if _, ok := item.(string); ok {
			t.Fatalf("catalog show repositories must be objects: %#v", item)
		}
	}
	listed := inventoryRepository(t, before, repo)
	if listed["profile"] != "missing" {
		t.Fatalf("unpublished source profile must be missing: %#v", listed)
	}
	if _, ok := listed["title"]; ok {
		t.Fatalf("missing profile must omit title: %#v", listed)
	}
	system := inventoryRepository(t, before, string(knowledge.SystemRepositoryID))
	if system["profile"] != "missing" {
		t.Fatalf("System Repository has no source profile: %#v", system)
	}
	if system["schemaCount"] != float64(len(knowledge.SystemSchemaOperations())) {
		t.Fatalf("System Repository schemaCount: %#v", system)
	}

	body(t, kc(home, "writer", "put", "--command-id", "source-profile-schema", "--repo", repo,
		"--object", string(knowledge.CoreSourceProfileSchemaV1),
		"--value", sourceProfileSchemaJSON(t)))
	body(t, kc(home, "writer", "put", "--command-id", "source-profile", "--repo", repo,
		"--object", string(knowledge.SourceProfileObjectID),
		"--schema-ref", string(knowledge.CoreSourceProfileSchemaV1),
		"--value", `{"title":"Payments warehouse","summary":"Published metrics and tables for payments."}`))

	after := asMap(t, body(t, kc(home, "catalog", "show")))
	present := inventoryRepository(t, after, repo)
	if present["profile"] != "present" || present["title"] != "Payments warehouse" ||
		present["summary"] != "Published metrics and tables for payments." {
		t.Fatalf("catalog show must include the published source profile: %#v", present)
	}
	if present["schemaCount"] != float64(1) {
		t.Fatalf("schemaCount must count schema/*: %#v", present)
	}

	listedRepos := asMap(t, body(t, kc(home, "catalog", "repository", "list")))
	listedPresent := inventoryRepository(t, listedRepos, repo)
	if listedPresent["profile"] != "present" || listedPresent["title"] != "Payments warehouse" {
		t.Fatalf("repository list must use the same inventory: %#v", listedPresent)
	}

	body(t, kc(home, "catalog", "workspace", "define", "--workspace", "payments", "--revision", "1",
		"--source", repo+"=refs/heads/main@knowledge"))
	workspace := asMap(t, body(t, kc(home, "catalog", "workspace", "show", "--workspace", "payments")))
	members, _ := workspace["repositories"].([]any)
	if len(members) != 1 || members[0] != repo {
		t.Fatalf("knowledge set members must remain source ids: %#v", workspace)
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
