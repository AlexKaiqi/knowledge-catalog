package cli_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/knowledge"
)

func TestLocalInitPublishesReadableImmutableSystemRepository(t *testing.T) {
	home := testkit.TempDir(t)
	catalogID := "kr://acme/system-test"
	initialized := asMap(t, body(t, kc(home, "init", "--catalog", catalogID)))
	system := asMap(t, initialized["system"])
	if system["repositoryId"] != string(knowledge.SystemRepositoryID) || system["metaSchema"] != string(knowledge.MetaSchemaV1) {
		t.Fatalf("init did not publish System Repository: %#v", initialized)
	}

	state := asMap(t, body(t, kc(home, "read", "--catalog", catalogID)))
	if !hasRepository(state, string(knowledge.SystemRepositoryID)) {
		t.Fatalf("System Repository is not registered: %#v", state)
	}

	body(t, kc(home, "local", "grant", "bootstrap", "--principal", "user:admin"))
	report := asMap(t, body(t, kc(home, "describe-schema", "--as", "agent:any",
		"--repo", string(knowledge.SystemRepositoryID), "--object", string(knowledge.MetaSchemaV1))))
	if len(report["schemas"].([]any)) != 1 {
		t.Fatalf("public System Schema read failed: %#v", report)
	}
	expectCode(t, kc(home, "put", "--as", "user:admin", "--command-id", "mutate-system",
		"--repo", string(knowledge.SystemRepositoryID), "--object", "schema/evil",
		"--value", `{"entity":"Evil"}`), "FORBIDDEN")
}

func TestLocalSystemPublishSeedsDoltAuthority(t *testing.T) {
	home := testkit.TempDir(t)
	body(t, kc(home, "init", "--catalog", "kr://acme/system-publish"))
	expectCode(t, kc(home, "local", "system", "publish"), "USAGE_INVALID")
	expectCode(t, kc(home, "local", "system", "publish", "--driver", "gitea"), "USAGE_INVALID")
	expectMsg(t, kc(home, "repo-add", "--repo", string(knowledge.SystemRepositoryID)), "immutable")

	published := asMap(t, body(t, kc(home, "local", "system", "publish", "--driver", "dolt")))
	if published["repositoryId"] != string(knowledge.SystemRepositoryID) || published["driver"] != "dolt" || published["seeded"] != true {
		t.Fatalf("first system publish should seed Dolt: %#v", published)
	}
	if published["metaSchemaDigest"] != string(knowledge.SystemMetaSchemaDigest()) {
		t.Fatalf("published digest drifted from the binary trust root: %#v", published)
	}
	replay := asMap(t, body(t, kc(home, "local", "system", "publish", "--driver", "dolt")))
	if replay["seeded"] != false || replay["commit"] != published["commit"] {
		t.Fatalf("second publish must verify without rewriting: %#v", replay)
	}

	body(t, kc(home, "local", "grant", "bootstrap", "--principal", "user:admin"))
	report := asMap(t, body(t, kc(home, "describe-schema", "--as", "agent:any",
		"--repo", string(knowledge.SystemRepositoryID), "--object", string(knowledge.MetaSchemaV1))))
	if len(report["schemas"].([]any)) != 1 {
		t.Fatalf("published System Schema is not readable: %#v", report)
	}
	status := asMap(t, body(t, kc(home, "status")))
	item := statusRepo(t, status, string(knowledge.SystemRepositoryID))
	if item["driver"] != "dolt" {
		t.Fatalf("reopened Home must use the published System authority: %#v", item)
	}
}
