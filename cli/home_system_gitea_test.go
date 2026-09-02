package cli_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"kc/internal/testkit"
	"kc/knowledge"
)

func TestLocalSystemPublishImportsBuiltinSchemasIntoLiveGitea(t *testing.T) {
	base, token, run := testkit.GiteaEndpoint(t)
	t.Setenv("KC_GITEA_TOKEN", token)
	home := testkit.TempDir(t)
	body(t, kc(home, "init", "--catalog", "kr://acme/system-gitea"))
	sum := sha256.Sum256([]byte("kr://kc/system" + run))
	dsn := base + "/kc/kc-system-" + hex.EncodeToString(sum[:8])

	published := asMap(t, body(t, kc(home, "local", "system", "publish", "--driver", "gitea", "--dsn", dsn)))
	if published["driver"] != "gitea" || published["dsn"] != dsn || published["seeded"] != true {
		t.Fatalf("live Gitea system publish should seed: %#v", published)
	}
	if published["metaSchemaDigest"] != string(knowledge.SystemMetaSchemaDigest()) {
		t.Fatalf("Gitea publication drifted from the binary trust root: %#v", published)
	}
	replay := asMap(t, body(t, kc(home, "local", "system", "publish", "--driver", "gitea", "--dsn", dsn)))
	if replay["seeded"] != false || replay["commit"] != published["commit"] {
		t.Fatalf("matching Gitea publication must verify: %#v", replay)
	}

	body(t, kc(home, "local", "grant", "bootstrap", "--principal", "user:admin"))
	report := asMap(t, body(t, kc(home, "describe-schema", "--as", "agent:any",
		"--repo", string(knowledge.SystemRepositoryID), "--object", string(knowledge.MetaSchemaV1))))
	if len(report["schemas"].([]any)) != 1 {
		t.Fatalf("Gitea System Schema is not readable after reopen: %#v", report)
	}
	expectCode(t, kc(home, "put", "--as", "user:admin", "--command-id", "mutate-gitea-system",
		"--repo", string(knowledge.SystemRepositoryID), "--object", "schema/evil",
		"--value", `{"entity":"Evil"}`), "FORBIDDEN")
}
