package cli_test

import (
	"strings"
	"testing"

	"kc/internal/testkit"
)

func TestCatalogAuditReadsSnapshotHistory(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	catID := "kr://kc/registry"

	started := asMap(t, body(t, kc(h, "init")))
	if _, ok := started["opsStream"]; ok {
		t.Fatal("init must not advertise an ops stream", started)
	}
	if _, present := started["catalog"]; present {
		t.Fatal(started)
	}
	expectCode(t, kc(h, "audit", "--as", "other"), "FORBIDDEN")
	expectCode(t, kc(h, "show", "--as", "other"), "FORBIDDEN")
	space := asMap(t, body(t, kc(h, "show")))
	if _, present := space["catalogId"]; present {
		t.Fatal(space)
	}

	trail := asMap(t, body(t, kc(h, "audit")))
	if trail["source"] != "registry" {
		t.Fatal(trail)
	}
	if !catalogLogHas(t, trail, "init") {
		t.Fatal("Catalog Snapshot history should record init", trail["entries"])
	}
	zero := asMap(t, body(t, kc(h, "audit", "--limit", "0")))
	if len(zero["entries"].([]any)) != len(trail["entries"].([]any)) {
		t.Fatalf("--limit 0 must mean the default audit page, not an unbounded export: %#v vs %#v", zero, trail)
	}
	expectCode(t, kc(h, "audit", "--limit", "201"), "USAGE_INVALID")

	seedRepo(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "policy/A", ""), "--command-id", "seed", "--value", `{"v":1}`))
	afterPut := asMap(t, body(t, kc(h, "audit")))
	if catalogLogHas(t, afterPut, "put") || catalogLogHas(t, afterPut, "COMMIT") {
		t.Fatal("knowledge writes must not land in Catalog Snapshot history", afterPut["entries"])
	}
	if !catalogLogHas(t, afterPut, "register") {
		t.Fatal(afterPut["entries"])
	}

	body(t, kc(h, "dataset", "define", "--dataset", "duty", "--revision", "1", "--source", core+"=refs/heads/main"))
	afterView := asMap(t, body(t, kc(h, "audit")))
	if !catalogLogHas(t, afterView, "dataset-define") {
		t.Fatal(afterView["entries"])
	}

	expectCode(t, kc(h, "put", testObjectReference(t, catID, "ops/note", ""), "--command-id", "catalog-is-not-a-repo", "--value", `{"note":"no"}`), "TARGET_REPOSITORY_DENIED")
}

func TestCatalogSnapshotStampsPrincipal(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"

	body(t, kc(h, "init"))
	seedRepo(t, h, core)
	rule := asMap(t, body(t, kc(h, "allow",
		"--principal", "agent:payments",
		"--action", "dataset.manage",
	)))
	body(t, kc(h, "dataset", "define",
		"--as", "agent:payments",
		"--request-id", "run-42",
		"--dataset", "duty",
		"--revision", "1",
		"--source", core+"=refs/heads/main",
	))
	var saw bool
	for _, item := range asMap(t, body(t, kc(h, "audit")))["entries"].([]any) {
		row := asMap(t, item)
		if !strings.HasPrefix(fmtString(row["message"]), "dataset-define") {
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

func TestRetiredCatalogCreationPreservesRegisteredInventory(t *testing.T) {
	h := testkit.TempDir(t)

	body(t, kc(h, "init"))
	expectCode(t, kc(h, "catalog-add"), "USAGE_INVALID")
	st := asMap(t, body(t, kc(h, "status")))
	if ids := businessRepositories(st); len(ids) != 0 || !hasRepository(st, "kr://kc/system") {
		t.Fatal("retired Catalog creation must preserve registered inventory", st["repositories"])
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
