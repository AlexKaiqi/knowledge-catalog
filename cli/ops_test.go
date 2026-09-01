package cli_test

import (
	"strings"
	"testing"

	"kc/internal/testkit"
)

func TestCatalogAuditIsGitLog(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	catID := "kr://acme/catalog"

	started := asMap(t, body(t, kc(h, "init", "--catalog", "kr://acme/catalog")))
	if _, ok := started["opsStream"]; ok {
		t.Fatal("init must not advertise an ops stream", started)
	}
	if started["catalog"] != catID {
		t.Fatal(started)
	}
	expectCode(t, kc(h, "audit", "--as", "other"), "FORBIDDEN")
	expectCode(t, kc(h, "read", "--catalog", "--as", "other"), "FORBIDDEN")
	space := asMap(t, body(t, kc(h, "read", "--catalog")))
	if space["catalogId"] != catID {
		t.Fatal(space)
	}

	trail := asMap(t, body(t, kc(h, "audit")))
	if trail["source"] != "catalog" || trail["catalogId"] != catID {
		t.Fatal(trail)
	}
	if !catalogLogHas(t, trail, "init") {
		t.Fatal("catalog git should record init", trail["entries"])
	}

	body(t, kc(h, "repo-add", "--repo", core))
	body(t, kc(h, "put",
		"--command-id", "seed",
		"--repo", core,
		"--object", "policy/A",
		"--value", `{"v":1}`,
	))
	afterPut := asMap(t, body(t, kc(h, "audit")))
	if catalogLogHas(t, afterPut, "put") || catalogLogHas(t, afterPut, "COMMIT") {
		t.Fatal("knowledge writes must not land in Catalog git", afterPut["entries"])
	}
	if !catalogLogHas(t, afterPut, "register") {
		t.Fatal(afterPut["entries"])
	}

	body(t, kc(h, "define-workspace", "--workspace", "duty", "--revision", "1", "--source", core+"=refs/heads/main"))
	afterView := asMap(t, body(t, kc(h, "audit")))
	if !catalogLogHas(t, afterView, "define-workspace") {
		t.Fatal(afterView["entries"])
	}

	expectCode(t, kc(h, "put", "--command-id", "catalog-is-not-a-repo", "--repo", catID,
		"--object", "ops/note", "--value", `{"note":"no"}`), "TARGET_REPOSITORY_DENIED")
}

func TestCatalogGitStampsPrincipal(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	catID := "kr://acme/catalog"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", core))
	rule := asMap(t, body(t, kc(h, "allow",
		"--principal", "agent:payments",
		"--cmd", "define-workspace",
		"--catalog", catID,
	)))
	body(t, kc(h, "define-workspace",
		"--as", "agent:payments",
		"--request-id", "run-42",
		"--catalog", catID,
		"--workspace", "duty",
		"--revision", "1",
		"--source", core+"=refs/heads/main",
	))
	var saw bool
	for _, item := range asMap(t, body(t, kc(h, "audit")))["entries"].([]any) {
		row := asMap(t, item)
		if !strings.HasPrefix(fmtString(row["message"]), "define-workspace") {
			continue
		}
		saw = true
		if row["author"] != "agent:payments" || row["requestId"] != "run-42" || row["ruleId"] != rule["id"] {
			t.Fatalf("%#v rule %#v", row, rule)
		}
	}
	if !saw {
		t.Fatal(asMap(t, body(t, kc(h, "audit")))["entries"])
	}
}

func TestCatalogLogNotInheritedBySecondCatalog(t *testing.T) {
	h := testkit.TempDir(t)
	docs := "kr://acme/docs/catalog"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "catalog-add", "--catalog", docs))
	st := asMap(t, body(t, kc(h, "status", "--catalog", docs)))
	if ids := businessRepositories(st); len(ids) != 0 || !hasRepository(st, "kr://kc/system") {
		t.Fatal("second Catalog must contain System but not inherit business repositories", st["repositories"])
	}
}

func catalogLogHas(t *testing.T, trail map[string]any, verb string) bool {
	t.Helper()
	raw, _ := trail["entries"].([]any)
	for _, item := range raw {
		row := asMap(t, item)
		msg, _ := row["message"].(string)
		got, _, _ := strings.Cut(msg, " ")
		if got == verb || msg == verb {
			return true
		}
	}
	return false
}

func fmtString(v any) string {
	s, _ := v.(string)
	return s
}
