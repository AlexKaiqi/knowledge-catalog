package cli_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func relationChangeSet(t *testing.T, repository string) string {
	t.Helper()
	payload := map[string]any{
		"targetRepository": repository,
		"targetRef":        "refs/heads/main",
		"operations": []any{map[string]any{
			"op": "PUT",
			"address": map[string]any{
				"kind": "Relation", "objectId": "relation:owned",
			},
			"value": map[string]any{
				"relationId": "relation:owned", "relationType": "owned-by", "direction": "DIRECTED",
				"endpoints": []any{
					map[string]any{"role": "subject", "objectRef": map[string]any{"repository": repository, "object": "Table:orders"}},
					map[string]any{"role": "owner", "objectRef": map[string]any{"repository": repository, "object": "Team:finance"}},
				},
			},
		}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func seedRelation(t *testing.T, home, repository string) {
	t.Helper()
	body(t, kc(home, "commit", "--command-id", "relation-seed", "--payload", relationChangeSet(t, repository)))
}

func relationHitID(t *testing.T, value any) string {
	t.Helper()
	hits, ok := asMap(t, value)["hits"].([]any)
	if !ok || len(hits) != 1 {
		t.Fatalf("want one relation hit, got %#v", value)
	}
	id, _ := asMap(t, hits[0])["objectId"].(string)
	return id
}

func TestRelationsWithoutIndexNeverFallsBackToAuthorityScan(t *testing.T) {
	home := testkit.TempDir(t)
	repository := "kr://acme/public/core"
	body(t, kc(home, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(home, "repo-add", "--repo", repository))
	seedRelation(t, home, repository)

	// Dolt remains the Canonical authority and exact object reads still work.
	body(t, kc(home, "read", "--repo", repository, "--object", "relation:owned"))
	// Candidate discovery has no authority fallback when the local profile has index:none.
	expectCode(t, kc(home, "relations", "--repo", repository, "--object", "Table:orders"), "CAPABILITY_UNSATISFIED")
}

func TestRelationRepositoryWorkspaceAndHTTPUseOneExactBasisExecutor(t *testing.T) {
	opensearchURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if opensearchURL == "" {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("KC_TEST_OPENSEARCH_URL is required")
		}
		t.Skip("run make test-e2e")
	}
	home := testkit.TempDir(t)
	repository := "kr://acme/public/core"
	body(t, kc(home, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(home, "store-set", "--index", "opensearch"))
	body(t, kc(home, "store-set", "--driver", "opensearch", "--url", opensearchURL))
	body(t, kc(home, "repo-add", "--repo", repository))
	seedRelation(t, home, repository)
	body(t, kc(home, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", repository+"=refs/heads/main@"))
	syncIndexes(t, home, repository)

	repositoryResult := body(t, kc(home, "relations", "--repo", repository, "--object", "Table:orders",
		"--relation-type", "owned-by", "--role", "subject", "--direction", "DIRECTED"))
	workspaceResult := body(t, kc(home, "relations", "--workspace", "agent", "--object", "kc://acme/public/core/Table:orders",
		"--relation-type", "owned-by", "--role", "subject", "--direction", "DIRECTED"))
	if repositoryID, workspaceID := relationHitID(t, repositoryResult), relationHitID(t, workspaceResult); repositoryID != workspaceID {
		t.Fatalf("repository/workspace relation executor drift: %q != %q", repositoryID, workspaceID)
	}

	body(t, kc(home, "allow", "--principal", "agent:http-test", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--workspace", "agent"))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--action", "knowledge.relations", "--repo", repository))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--action", "knowledge.read", "--repo", repository))
	handler := cli.HTTPHandler(home)
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	httpResult := postAny(t, server.URL, "/knowledge/v1/relations:query", map[string]any{
		"workspace": "agent", "endpoint": "kc://acme/public/core/Table:orders",
		"relationType": "owned-by", "role": "subject", "direction": "DIRECTED",
	})
	if httpID, cliID := relationHitID(t, httpResult), relationHitID(t, workspaceResult); httpID != cliID {
		t.Fatalf("HTTP/CLI relation executor drift: %q != %q", httpID, cliID)
	}
}
