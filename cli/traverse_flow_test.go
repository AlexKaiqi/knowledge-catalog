package cli_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
	"kc/observability"
)

// traverseChangeSet seeds a two-hop chain plus one edge that points outside
// every traversal scope: orders -> finance -> me, and orders -> guest in a
// repository that is never attached to the dataset.
func traverseChangeSet(t *testing.T, repository, host string) string {
	t.Helper()
	payload := map[string]any{
		"targetRepository": repository,
		"targetRef":        "refs/heads/main",
		"operations": []any{
			map[string]any{
				"op": "PUT", "address": map[string]any{"kind": "Relation", "objectId": "relation:owned"},
				"value": map[string]any{
					"relationId": "relation:owned", "relationType": "owned-by", "direction": "DIRECTED",
					"endpoints": []any{
						map[string]any{"role": "subject", "objectRef": map[string]any{"repository": repository, "object": "Table:orders"}},
						map[string]any{"role": "owner", "objectRef": map[string]any{"repository": repository, "object": "Team:finance"}},
					},
				},
			},
			map[string]any{
				"op": "PUT", "address": map[string]any{"kind": "Relation", "objectId": "relation:reported"},
				"value": map[string]any{
					"relationId": "relation:reported", "relationType": "owned-by", "direction": "DIRECTED",
					"endpoints": []any{
						map[string]any{"role": "subject", "objectRef": map[string]any{"repository": repository, "object": "Team:finance"}},
						map[string]any{"role": "owner", "objectRef": map[string]any{"repository": repository, "object": "Person:me"}},
					},
				},
			},
			map[string]any{
				"op": "PUT", "address": map[string]any{"kind": "Relation", "objectId": "relation:shared"},
				"value": map[string]any{
					"relationId": "relation:shared", "relationType": "owned-by", "direction": "DIRECTED",
					"endpoints": []any{
						map[string]any{"role": "subject", "objectRef": map[string]any{"repository": repository, "object": "Table:orders"}},
						map[string]any{"role": "owner", "objectRef": map[string]any{"repository": host, "object": "Team:guest"}},
					},
				},
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func traverseNodes(t *testing.T, value any) map[string]float64 {
	t.Helper()
	nodes, ok := asMap(t, value)["nodes"].([]any)
	if !ok {
		t.Fatalf("traverse page without nodes: %#v", value)
	}
	out := map[string]float64{}
	for _, node := range nodes {
		fields := asMap(t, node)
		id, _ := fields["objectId"].(string)
		depth, _ := fields["depth"].(float64)
		out[id] = depth
	}
	return out
}

func traverseEdgeIDs(t *testing.T, value any) []string {
	t.Helper()
	edges, _ := asMap(t, value)["edges"].([]any)
	ids := []string{}
	for _, edge := range edges {
		id, _ := asMap(t, edge)["objectId"].(string)
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func traverseBoundary(t *testing.T, value any) []string {
	t.Helper()
	boundaries, _ := asMap(t, value)["boundary"].([]any)
	out := []string{}
	for _, item := range boundaries {
		fields := asMap(t, item)
		id, _ := fields["objectId"].(string)
		if reason, _ := fields["reason"].(string); reason != "outside-scope" {
			t.Fatalf("unexpected boundary reason %q for %s", reason, id)
		}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func TestTraverseNeighborhoodClosureAcrossChannels(t *testing.T) {
	opensearchURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if opensearchURL == "" {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("KC_TEST_OPENSEARCH_URL is required")
		}
		t.Skip("run make test")
	}
	home := testkit.TempDir(t)
	repository := "kr://acme/public/core"
	host := "kr://acme/partner"
	body(t, kc(home, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(home, "store-set", "--index", "opensearch"))
	body(t, kc(home, "store-set", "--driver", "opensearch", "--url", opensearchURL))
	seedRepo(t, home, repository)
	body(t, kc(home, "commit", "--command-id", "relation-chain", "--payload", traverseChangeSet(t, repository, host)))
	body(t, kc(home, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", repository+"=refs/heads/main@"))
	syncIndexes(t, home, repository)

	repositoryFull := body(t, kc(home, "traverse", "--repo", repository, "--object", "Table:orders",
		"--max-hops", "2", "--limit", "100", "--relation-type", "owned-by", "--direction", "DIRECTED"))
	// The closure is a deduplicated neighborhood: the start is never a node,
	// depth-2 nodes only appear when the hop ceiling allows a second hop, and
	// the out-of-scope endpoint is an explicit boundary, not a node.
	nodes := traverseNodes(t, repositoryFull)
	if len(nodes) != 2 || nodes["Team:finance"] != 1 || nodes["Person:me"] != 2 {
		t.Fatalf("repository closure nodes: %#v", nodes)
	}
	if got := traverseEdgeIDs(t, repositoryFull); len(got) != 3 || strings.Join(got, ",") != "relation:owned,relation:reported,relation:shared" {
		t.Fatalf("repository closure edges: %#v", got)
	}
	if got := traverseBoundary(t, repositoryFull); len(got) != 1 || got[0] != "Team:guest" {
		t.Fatalf("out-of-scope frontier must be a boundary: %#v", got)
	}

	repositoryOneHop := body(t, kc(home, "traverse", "--repo", repository, "--object", "Table:orders",
		"--max-hops", "1", "--relation-type", "owned-by", "--direction", "DIRECTED"))
	if nodes := traverseNodes(t, repositoryOneHop); len(nodes) != 1 || nodes["Team:finance"] != 1 {
		t.Fatalf("one-hop ceiling: %#v", nodes)
	}
	expectCode(t, kc(home, "traverse", "--repo", repository, "--object", "Table:orders", "--max-hops", "0"), "USAGE_INVALID")
	expectCode(t, kc(home, "traverse", "--repo", repository, "--object", "Table:orders", "--max-hops", "9"), "USAGE_INVALID")
	expectCode(t, kc(home, "traverse", "--repo", repository, "--object", "Table:orders"), "USAGE_INVALID")

	datasetFull := body(t, kc(home, "traverse", "--dataset", "agent", "--object", "kc://acme/public/core/Table:orders",
		"--max-hops", "2", "--relation-type", "owned-by", "--direction", "DIRECTED"))
	datasetNodes := traverseNodes(t, datasetFull)
	if len(datasetNodes) != len(nodes) {
		t.Fatalf("repository/workspace closure drift: %#v != %#v", datasetNodes, nodes)
	}

	// Delta paging: --limit 1 yields disjoint pages whose union is the closure.
	request := []string{"traverse", "--dataset", "agent", "--object", "kc://acme/public/core/Table:orders",
		"--max-hops", "2", "--limit", "1", "--relation-type", "owned-by", "--direction", "DIRECTED"}
	union := map[string]float64{}
	pages := 0
	for {
		if pages > 8 {
			t.Fatal("traverse pagination did not terminate")
		}
		page := body(t, kc(home, request...))
		for id, depth := range traverseNodes(t, page) {
			if _, seen := union[id]; seen {
				t.Fatalf("node %s appears in two pages", id)
			}
			union[id] = depth
		}
		exhausted, _ := asMap(t, page)["exhausted"].(bool)
		pages++
		if exhausted {
			break
		}
		cursor, _ := asMap(t, page)["continuation"].(string)
		if cursor == "" {
			t.Fatal("non-exhausted traverse page without continuation")
		}
		request = append(request, "--continuation", cursor)
	}
	if pages < 3 || len(union) != 2 || union["Team:finance"] != 1 || union["Person:me"] != 2 {
		t.Fatalf("paged closure union: pages=%d %#v", pages, union)
	}

	body(t, kc(home, "allow", "--principal", "agent:http-test", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--dataset", "agent"))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--action", "knowledge.traverse", "--repo", repository))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--action", "knowledge.read", "--repo", repository))
	handler := cli.HTTPHandler(home)
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	httpResult := postAny(t, server.URL, "/knowledge/v1/traverse:query", map[string]any{
		"dataset": "agent", "endpoint": "kc://acme/public/core/Table:orders",
		"maxHops": 2, "relationType": "owned-by", "direction": "DIRECTED",
	})
	httpNodes := traverseNodes(t, httpResult)
	if len(httpNodes) != 2 || httpNodes["Team:finance"] != 1 || httpNodes["Person:me"] != 2 {
		t.Fatalf("HTTP closure nodes: %#v", httpNodes)
	}
	page, err := observability.NewFileStore(home).Access(context.Background(), observability.AccessQuery{
		Action: "knowledge.traverse", Limit: 10,
	})
	if err != nil || len(page.Entries) == 0 {
		t.Fatalf("traverse access evidence: %d entries err=%v", len(page.Entries), err)
	}
}
