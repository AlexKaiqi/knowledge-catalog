package cli_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"kc/internal/testkit"
)

// knowledge.md U2/U5/U6 写入面旅程：切面级修改保持兄弟切面与对象身份、
// 提交响应丢失后经回执恢复且同一命令身份不能携带另一份内容、跨仓关系
// 发布不写端点仓也不扩大读权。

// U2：只修改一个切面，兄弟切面保持原值，对象身份不变。
func TestAspectModificationKeepsSiblingAndIdentity(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	seedRepo(t, h, "kr://acme/public/core")
	body(t, kc(h, "put", "--command-id", "def-v1", "--repo", "kr://acme/public/core",
		"--object", "Policy:P-103", "--aspect", "definition", "--value", `{"text":"v1"}`))
	body(t, kc(h, "put", "--command-id", "run-v1", "--repo", "kr://acme/public/core",
		"--object", "Policy:P-103", "--aspect", "runtime", "--value", `{"status":"on"}`))

	body(t, kc(h, "put", "--command-id", "def-v2", "--repo", "kr://acme/public/core",
		"--object", "Policy:P-103", "--aspect", "definition", "--value", `{"text":"v2"}`))

	definition := asMap(t, body(t, kc(h, "read", "--repo", "kr://acme/public/core", "--object", "Policy:P-103", "--aspect", "definition")))
	runtime := asMap(t, body(t, kc(h, "read", "--repo", "kr://acme/public/core", "--object", "Policy:P-103", "--aspect", "runtime")))
	if got := definition["value"]; !reflect.DeepEqual(got, map[string]any{"text": "v2"}) {
		t.Fatalf("modified aspect must serve v2: %#v", got)
	}
	if got := runtime["value"]; !reflect.DeepEqual(got, map[string]any{"status": "on"}) {
		t.Fatalf("sibling aspect must keep its value: %#v", got)
	}
	for _, hit := range []map[string]any{definition, runtime} {
		if hit["objectId"] != "Policy:P-103" {
			t.Fatalf("aspect identity must stay on the same object: %#v", hit)
		}
		if hit["aspectName"] != "definition" && hit["aspectName"] != "runtime" {
			t.Fatalf("read must name the aspect: %#v", hit)
		}
	}
}

// U5：同一命令身份携带另一份内容必须被拒绝；响应丢失后经回执恢复已接受事实。
func TestLostResponseRecoversFromReceiptAndRefusesContentSwap(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	seedRepo(t, h, "kr://acme/public/core")
	accepted := asMap(t, body(t, kc(h, "put", "--command-id", "sync-1", "--repo", "kr://acme/public/core",
		"--object", "policy/A", "--value", `{"v":1}`)))
	if accepted["disposition"] != "APPLIED" {
		t.Fatalf("first publish must apply: %#v", accepted)
	}
	head := body(t, kc(h, "writer", "head", "--repo", "kr://acme/public/core"))

	// 同一命令身份、另一份内容：拒绝，HEAD 不动。
	expectCode(t, kc(h, "put", "--command-id", "sync-1", "--repo", "kr://acme/public/core",
		"--object", "policy/A", "--value", `{"v":2}`), "IDEMPOTENCY_CONFLICT")
	if got := body(t, kc(h, "writer", "head", "--repo", "kr://acme/public/core")); !reflect.DeepEqual(got, head) {
		t.Fatalf("content swap must not move HEAD: %v vs %v", got, head)
	}

	// 响应丢失：回执暴露已接受事实，无需重写。
	replay := asMap(t, body(t, kc(h, "put", "--command-id", "sync-1", "--repo", "kr://acme/public/core",
		"--object", "policy/A", "--value", `{"v":1}`)))
	if replay["disposition"] != "REPLAYED" {
		t.Fatalf("identical retry must replay: %#v", replay)
	}
	receipt := asMap(t, body(t, kc(h, "writer", "receipt", "--command-id", "sync-1")))
	if receipt["status"] != "APPLIED" {
		t.Fatalf("receipt must expose the accepted fact: %#v", receipt)
	}
	if _, ok := receipt["digest"]; !ok {
		t.Fatalf("receipt must carry the payload identity digest: %#v", receipt)
	}
	if nested := asMap(t, receipt["receipt"]); nested["disposition"] != "APPLIED" {
		t.Fatalf("receipt must carry the commit disposition: %#v", nested)
	}
}

// U6：在有写权的仓发布关系、端点指向另一仓：发布不写端点仓、不扩大读权，
// 关系对象保留端点坐标与存储来源。
func TestCrossRepositoryRelationPublishKeepsEndpointRepoUntouched(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	seedRepo(t, h, "kr://acme/public/core")
	seedRepo(t, h, "kr://acme/other/teams")
	otherHead := body(t, kc(h, "writer", "head", "--repo", "kr://acme/other/teams"))
	grants := body(t, kc(h, "grant", "list"))

	payload := map[string]any{
		"targetRepository": "kr://acme/public/core",
		"targetRef":        "refs/heads/main",
		"operations": []any{map[string]any{
			"op":      "PUT",
			"address": map[string]any{"kind": "Relation", "objectId": "relation:owned"},
			"value": map[string]any{
				"relationId": "relation:owned", "relationType": "owned-by", "direction": "DIRECTED",
				"endpoints": []any{
					map[string]any{"role": "subject", "objectRef": map[string]any{"repository": "kr://acme/public/core", "object": "Table:orders"}},
					map[string]any{"role": "owner", "objectRef": map[string]any{"repository": "kr://acme/other/teams", "object": "Team:finance"}},
				},
			},
		}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	body(t, kc(h, "commit", "--command-id", "relation-cross", "--payload", string(raw)))

	if got := body(t, kc(h, "writer", "head", "--repo", "kr://acme/other/teams")); !reflect.DeepEqual(got, otherHead) {
		t.Fatalf("relation publish must not write the endpoint repo: %v vs %v", got, otherHead)
	}
	if got := body(t, kc(h, "grant", "list")); !reflect.DeepEqual(got, grants) {
		t.Fatalf("relation publish must not widen grants: %#v", got)
	}
	relation := asMap(t, body(t, kc(h, "read", "--repo", "kr://acme/public/core", "--object", "relation:owned")))
	rawValue, err := json.Marshal(relation["value"])
	if err != nil {
		t.Fatal(err)
	}
	value := map[string]any{}
	if err := json.Unmarshal(rawValue, &value); err != nil {
		t.Fatal(err)
	}
	endpoints, ok := value["endpoints"].([]any)
	if !ok || len(endpoints) != 2 {
		t.Fatalf("relation must keep both endpoint coordinates: %#v", value)
	}
	owner := asMap(t, endpoints[1])["objectRef"].(map[string]any)
	if owner["repository"] != "kr://acme/other/teams" || owner["object"] != "Team:finance" {
		t.Fatalf("cross-repo endpoint must keep its storage source: %#v", owner)
	}
}
