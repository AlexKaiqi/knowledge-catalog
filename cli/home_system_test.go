package cli_test

import (
	"testing"

	apphome "kc/home"
	"kc/internal/testkit"
	objectmodel "kc/object"
)

func TestLocalInitPublishesReadableImmutableSystemRepository(t *testing.T) {
	home := testkit.TempDir(t)

	initialized := asMap(t, body(t, kc(home, "init")))
	system := asMap(t, initialized["system"])
	if system["repositoryId"] != string(objectmodel.SystemRepositoryID) || system["metaSchema"] != string(objectmodel.MetaSchemaV1) {
		t.Fatalf("init did not publish System Repository: %#v", initialized)
	}

	state := asMap(t, body(t, kc(home, "show")))
	if !hasRepository(state, string(objectmodel.SystemRepositoryID)) {
		t.Fatalf("System Repository is not registered: %#v", state)
	}

	body(t, kc(home, "local", "grant", "bootstrap", "--principal", "admin"))
	report := asMap(t, body(t, kc(home, "describe-schema", "--as", "agent:any",
		"--repo", string(objectmodel.SystemRepositoryID), "--object", string(objectmodel.MetaSchemaV1))))
	if len(report["schemas"].([]any)) != 1 {
		t.Fatalf("public System Schema read failed: %#v", report)
	}
	expectCode(t, kc(home, "put", testObjectReference(t, string(objectmodel.SystemRepositoryID), "schema/evil", ""), "--as", "admin", "--command-id", "mutate-system", "--value", `{"entity":"Evil"}`), "FORBIDDEN")
}

func TestLocalSystemPublishSeedsLakeFSAuthority(t *testing.T) {
	fake := testkit.NewLakeFSFake(t)
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	home := testkit.TempDir(t)
	if _, _, err := apphome.InitHome(home, apphome.RegistryBinding{ID: "kr://kc/registry", Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())}); err != nil {
		t.Fatal(err)
	}
	fakeDSN := fake.DSN(fake.NewRepo())
	expectCode(t, kc(home, "local", "system", "publish"), "USAGE_INVALID")
	expectCode(t, kc(home, "local", "system", "publish", "--driver", "gitea"), "USAGE_INVALID")

	published := asMap(t, body(t, kc(home, "local", "system", "publish", "--driver", "lakefs", "--dsn", fakeDSN)))
	if published["repositoryId"] != string(objectmodel.SystemRepositoryID) || published["driver"] != "lakefs" || published["seeded"] != true {
		t.Fatalf("first system publish should seed lakeFS: %#v", published)
	}
	if published["metaSchemaDigest"] != string(objectmodel.SystemMetaSchemaDigest()) {
		t.Fatalf("published digest drifted from the binary trust root: %#v", published)
	}
	replay := asMap(t, body(t, kc(home, "local", "system", "publish", "--driver", "lakefs", "--dsn", fakeDSN)))
	if replay["seeded"] != false || replay["commit"] != published["commit"] {
		t.Fatalf("second publish must verify without rewriting: %#v", replay)
	}

	expectMsg(t, kc(home, "repo-add", "--repo", string(objectmodel.SystemRepositoryID)), "immutable")
	body(t, kc(home, "init"))

	body(t, kc(home, "local", "grant", "bootstrap", "--principal", "admin"))
	report := asMap(t, body(t, kc(home, "describe-schema", "--as", "agent:any",
		"--repo", string(objectmodel.SystemRepositoryID), "--object", string(objectmodel.MetaSchemaV1))))
	if len(report["schemas"].([]any)) != 1 {
		t.Fatalf("published System Schema is not readable: %#v", report)
	}
	status := asMap(t, body(t, kc(home, "status")))
	item := statusRepo(t, status, string(objectmodel.SystemRepositoryID))
	if item["driver"] != "lakefs" {
		t.Fatalf("reopened Home must use the published System authority: %#v", item)
	}
}
