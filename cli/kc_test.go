package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func kc(home string, args ...string) kcRunResult {
	args = groupedTestArgs(args)
	all := append([]string{"--home", home}, args...)
	return kcRunResultFrom(cli.RunEmbeddedForTest(all, nil), publicCommandPath(all))
}

func kcRemote(serverURL, principal string, args ...string) kcRunResult {
	args = groupedTestArgs(args)
	all := append([]string{"--server", serverURL, "--as", principal}, args...)
	return kcRunResultFrom(cli.Run(all), publicCommandPath(all))
}

func publicCommandPath(all []string) string {
	parsed, err := cli.ParseArgs(all)
	if err != nil {
		return ""
	}
	for n := len(parsed.Args); n >= 0; n-- {
		path := strings.Join(append([]string{parsed.Command}, parsed.Args[:n]...), " ")
		if cli.CLICommandForTest(path) {
			return path
		}
	}
	return ""
}

func kcRunResultFrom(run cli.RunResult, operation string) kcRunResult {
	result := kcRunResult{RunResult: run, operation: operation, runID: runSequence.Add(1)}
	recordCommandRun(operation, result.Status == 0)
	return result
}

// kcClientLocal runs a public command that keeps state in the client credential
// store rather than a Home. `kc login`/`logout` reject --home together with a
// Server target, so they cannot go through kc(), which always injects --home.
// Coverage is recorded identically so the per-command gate still applies.
func kcClientLocal(args ...string) kcRunResult {
	args = groupedTestArgs(args)
	operation := ""
	if parsed, err := cli.ParseArgs(args); err == nil {
		for n := len(parsed.Args); n >= 0; n-- {
			path := strings.Join(append([]string{parsed.Command}, parsed.Args[:n]...), " ")
			if cli.CLICommandForTest(path) {
				operation = path
				break
			}
		}
	}
	result := kcRunResult{RunResult: cli.RunEmbeddedForTest(args, nil), operation: operation, runID: runSequence.Add(1)}
	recordCommandRun(operation, result.Status == 0)
	return result
}

// Retrieval is an explicitly maintained projection. Consumer operations never
// build it as a side effect, so journeys that expect SEARCH/RELATIONS first
// publish the required exact-basis projection.
func syncIndexes(t *testing.T, home string, repositories ...string) {
	t.Helper()
	for _, repository := range repositories {
		body(t, kc(home, "index-sync", "--repo", repository))
	}
}

// Existing package tests name the internal operation they exercise. This
// adapter is test-only: the process entry still rejects every flat command.
func groupedTestArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	args = append([]string{}, args...)
	if args[0] == "allow" || args[0] == "allowed" {
		for i := 1; i+1 < len(args); i++ {
			if args[i] == "--cmd" {
				args[i] = "--action"
				args[i+1] = legacyTestActions(args[i+1])
			}
		}
	}
	if args[0] == "hook-add" {
		for i := 1; i+1 < len(args); i++ {
			if args[i] == "--on" {
				args[i+1] = legacyTestActions(args[i+1])
			}
		}
	}
	if args[0] == "read" {
		catalogView := false
		knowledgeTarget := false
		for _, arg := range args[1:] {
			if arg == "--catalog" || strings.HasPrefix(arg, "--catalog=") {
				catalogView = true
			}
			if arg == "--workspace" || strings.HasPrefix(arg, "--workspace=") ||
				arg == "--repo" || strings.HasPrefix(arg, "--repo=") ||
				arg == "--object" || strings.HasPrefix(arg, "--object=") {
				knowledgeTarget = true
			}
		}
		if catalogView && !knowledgeTarget {
			return append([]string{"catalog", "show"}, args[1:]...)
		}
	}
	paths := map[string][]string{
		"init": {"local", "init"}, "status": {"local", "status"},
		"catalog-add": {"local", "catalog", "attach"}, "repo-add": {"local", "repository", "attach"},
		"store-ls": {"local", "store", "show"}, "store-set": {"local", "store", "set"}, "overlay": {"local", "workspace", "overlay"},
		"whoami": {"identity", "whoami"}, "allow": {"admin", "grant", "add"}, "revoke": {"admin", "grant", "remove"}, "allowed": {"admin", "grant", "list"},
		"audit": {"catalog", "audit"}, "register": {"catalog", "repository", "register"}, "archive-repo": {"catalog", "repository", "archive"},
		"archive-catalog":  {"catalog", "archive"},
		"define-workspace": {"catalog", "workspace", "define"}, "retire-workspace": {"catalog", "workspace", "retire"}, "resolve": {"catalog", "workspace", "resolve"},
		"read": {"knowledge", "read"}, "search": {"knowledge", "search"}, "relations": {"knowledge", "relations"}, "provenance": {"knowledge", "provenance"},
		"log": {"knowledge", "log"}, "describe-schema": {"knowledge", "schema", "describe"}, "resolve-binding": {"knowledge", "binding", "resolve"},
		"put": {"writer", "put"}, "remove": {"writer", "remove"}, "commit": {"writer", "commit"}, "ingest": {"writer", "ingest"}, "writer-head": {"writer", "head"}, "receipt": {"writer", "receipt"},
		"propose": {"governance", "proposal", "create"}, "merge": {"governance", "proposal", "merge"}, "preview": {"governance", "preview", "create"},
		"validate": {"governance", "preview", "validate"}, "record-validation": {"governance", "validation", "record"},
		"describe-index": {"operations", "projection", "describe"}, "index-sync": {"operations", "projection", "sync"}, "describe-access": {"operations", "access", "describe"},
		"hook-add": {"operations", "hook", "add"}, "hook-ls": {"operations", "hook", "list"}, "hook-rm": {"operations", "hook", "remove"},
		"gate-add": {"operations", "gate", "add"}, "gate-ls": {"operations", "gate", "list"}, "gate-rm": {"operations", "gate", "remove"},
		"access-log": {"operations", "audit", "access"}, "trace": {"operations", "audit", "trace"}, "hitmap": {"operations", "audit", "hitmap"}, "record-feedback": {"operations", "feedback", "record"},
		"diff": {"maintenance", "object", "diff"}, "inspect": {"maintenance", "workspace", "inspect"}, "checkout": {"maintenance", "workspace", "checkout"}, "sync": {"maintenance", "workspace", "sync"},
	}
	if path := paths[args[0]]; len(path) > 0 {
		return append(append([]string{}, path...), args[1:]...)
	}
	return args
}

func legacyTestActions(raw string) string {
	legacy := map[string]string{
		"put": "writer.commit", "remove": "writer.commit", "commit": "writer.commit",
		"read": "knowledge.read", "read-workspace": "workspace.consume", "read-catalog": "catalog.read", "search": "knowledge.search",
		"relations": "knowledge.relations", "resolve": "workspace.resolve", "inspect": "maintenance.workspace.inspect",
		"describe-access": "knowledge.access.describe", "ingest": "writer.preview",
		"propose": "governance.proposal.create", "preview": "governance.preview.create",
		"validate": "governance.validate", "record-validation": "governance.validation.record", "merge": "governance.merge",
		"define-workspace": "workspace.manage", "retire-workspace": "workspace.manage",
		"register": "catalog.repositories.manage", "archive-repo": "catalog.repositories.manage", "archive-catalog": "catalog.manage",
	}
	parts := strings.Split(raw, ",")
	for i, part := range parts {
		if action := legacy[strings.TrimSpace(part)]; action != "" {
			parts[i] = action
		}
	}
	return strings.Join(parts, ",")
}

func body(t *testing.T, result kcRunResult) any {
	t.Helper()
	if result.Status != 0 {
		t.Fatalf("status %d stdout %s", result.Status, result.Stdout)
	}
	var value any
	if err := json.Unmarshal([]byte(result.Stdout), &value); err != nil {
		t.Fatal(err, result.Stdout)
	}
	recordAssertedSuccess(t, result)
	return value
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("not object: %#v", value)
	}
	return m
}

func failError(t *testing.T, result kcRunResult) map[string]any {
	t.Helper()
	if result.Status != 1 {
		t.Fatalf("want status 1, got %d stdout %s", result.Status, result.Stdout)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		t.Fatal(err, result.Stdout)
	}
	return asMap(t, payload["error"])
}

func expectCode(t *testing.T, result kcRunResult, code string) {
	t.Helper()
	err := failError(t, result)
	if err["code"] != code {
		t.Fatalf("want error code %s, got %#v", code, err)
	}
	recordAssertedFailure(t, result, code)
}

func expectMsg(t *testing.T, result kcRunResult, substr string) {
	t.Helper()
	err := failError(t, result)
	msg, _ := err["message"].(string)
	if !strings.Contains(msg, substr) {
		t.Fatalf("want message containing %q, got %#v", substr, err)
	}
	code, _ := err["code"].(string)
	recordAssertedFailure(t, result, code)
}

func TestParseSkipsBareDashDash(t *testing.T) {
	parsed, err := cli.ParseArgs([]string{"--", "serve", "--home", "/tmp/kc-demo", "--listen", "127.0.0.1:7380"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Command != "serve" {
		t.Fatal(parsed)
	}
	if cli.FlagString(parsed.Flags, "home") != "/tmp/kc-demo" {
		t.Fatal(parsed.Flags)
	}
}

func TestHelp(t *testing.T) {
	result := cli.Run([]string{"help"})
	if result.Status != 0 {
		t.Fatal(result)
	}
	want := cli.Help
	if !strings.HasSuffix(want, "\n") {
		want += "\n"
	}
	if result.Stdout != want {
		t.Fatalf("help mismatch")
	}
	for _, needle := range []string{"kc writer put", "kc catalog show", "kc knowledge binding resolve", "kc operations access describe", "kcfs for lazy files", "kc governance preview validate", "kc knowledge log", "kc catalog audit", "kc operations hook", "kc operations gate", "kc serve", "kc local store set"} {
		if !strings.Contains(result.Stdout, needle) {
			t.Fatal(needle)
		}
	}
	if strings.Contains(result.Stdout, "alias of") || strings.Contains(result.Stdout, "kc read-release") || strings.Contains(result.Stdout, "kc read-catalog") {
		t.Fatal("help must not advertise command aliases")
	}
}

func TestRoleHelp(t *testing.T) {
	for topic, needles := range map[string][]string{
		"consumer": {"workspace resolve", "knowledge search", "never enumerates", "--pin", "--source <id>"},
		"provider": {"writer put", "writer ingest", "Collectors remain outside KC", "Schema is versioned knowledge"},
		"governor": {"workspace define", "grant add", "proposal merge"},
	} {
		result := cli.Run([]string{"help", topic})
		if result.Status != 0 {
			t.Fatalf("help %s: %#v", topic, result)
		}
		for _, needle := range needles {
			if !strings.Contains(result.Stdout, needle) {
				t.Fatalf("help %s missing %q: %s", topic, needle, result.Stdout)
			}
		}
		if topic == "consumer" || topic == "provider" {
			for _, leak := range []string{"refs/heads", "--home", "local repository attach", "OpenSearch", "Dolt", "Gitea", "kc local"} {
				if strings.Contains(result.Stdout, leak) {
					t.Fatalf("help %s leaked %q: %s", topic, leak, result.Stdout)
				}
			}
		}
	}
	unknown := cli.Run([]string{"help", "operator"})
	if unknown.Status == 0 || !strings.Contains(unknown.Stdout, "unknown help topic") {
		t.Fatalf("unknown role help must fail clearly: %#v", unknown)
	}
	extra := cli.Run([]string{"help", "consumer", "extra"})
	if extra.Status == 0 || !strings.Contains(extra.Stdout, "consumer extra") {
		t.Fatalf("role help must reject extra positionals: %#v", extra)
	}
}

func TestProtocolErrorJSON(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	expectCode(t, kc(h, "read", "--repo", "kr://acme/public/core", "--object", "missing", "--ref", "refs/heads/main"), "KNOWLEDGE_REF_UNRESOLVED")
}

func TestProposeMergeIsVisibleOnView(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	body(t, kc(h, "put", "--command-id", "seed", "--repo", "kr://acme/public/core", "--object", "policy/P-103", "--value", `{"v":1}`))
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"))
	proposal := asMap(t, body(t, kc(h,
		"propose", "--proposal-id", "PR-1", "--repo", "kr://acme/public/core",
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-1",
		"--object", "policy/P-103", "--value", `{"v":2}`,
	)))
	preview := asMap(t, body(t, kc(h, "preview", "--proposal", "PR-1", "--workspace", "agent")))
	structural := asMap(t, body(t, kc(h, "validate", "--preview", preview["previewId"].(string))))
	if structural["outcome"] != "PASSED" {
		t.Fatal(structural)
	}
	validation := asMap(t, body(t, kc(h, "record-validation", "--preview", preview["previewId"].(string), "--suite", "S7", "--outcome", "PASSED")))
	merged := asMap(t, body(t, kc(h, "merge", "--proposal", proposal["proposalId"].(string), "--preview", preview["previewId"].(string), "--validation", validation["reportId"].(string))))
	if merged["commitId"] != proposal["candidateCommit"] {
		t.Fatal(merged, proposal)
	}
	serving := body(t, kc(h, "read", "--workspace", "agent", "--object", "policy/P-103")).([]any)
	if asMap(t, serving[0])["value"].(map[string]any)["v"] != float64(2) {
		t.Fatal(serving)
	}
}

func TestMultipleCatalogs(t *testing.T) {
	h := testkit.TempDir(t)
	started := asMap(t, body(t, kc(h, "init", "--catalog", "kr://acme/catalog")))
	if started["catalog"] != "kr://acme/catalog" {
		t.Fatal(started)
	}
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	body(t, kc(h, "put",
		"--command-id", "seed",
		"--repo", "kr://acme/public/core",
		"--object", "policy/P-1",
		"--value", `{"v":1}`,
	))
	added := asMap(t, body(t, kc(h, "catalog-add", "--catalog", "kr://acme/docs/catalog")))
	if added["catalog"] != "kr://acme/docs/catalog" {
		t.Fatal(added)
	}
	expectMsg(t, kc(h, "catalog-add", "--catalog", "kr://acme/docs/catalog"), "already exists")
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/docs/catalog"), "reserved")
	body(t, kc(h, "define-workspace", "--workspace", "ops", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"))
	body(t, kc(h, "register", "--catalog", "kr://acme/docs/catalog", "--repo", "kr://acme/public/core"))
	body(t, kc(h, "define-workspace",
		"--catalog", "kr://acme/docs/catalog",
		"--workspace", "docs",
		"--revision", "1",
		"--source", "kr://acme/public/core=refs/heads/main",
	))
	status := asMap(t, body(t, kc(h, "status")))
	catalogs := status["catalogs"].([]any)
	if len(catalogs) != 2 {
		t.Fatal(status["catalogs"])
	}
	ids := map[string]bool{}
	for _, item := range catalogs {
		ids[asMap(t, item)["id"].(string)] = true
	}
	if !ids["kr://acme/catalog"] || !ids["kr://acme/docs/catalog"] {
		t.Fatal(status["catalogs"])
	}
	if _, ok := status["releases"]; ok {
		t.Fatal("status must not list releases", status["releases"])
	}
	other := asMap(t, body(t, kc(h, "status", "--catalog", "kr://acme/docs/catalog")))
	if asMap(t, other["catalog"])["repositoryId"] != "kr://acme/docs/catalog" {
		t.Fatal(other["catalog"])
	}
	sawDocs, sawOps := false, false
	for _, item := range other["workspaces"].([]any) {
		switch asMap(t, item)["workspaceId"] {
		case "docs":
			sawDocs = true
		case "ops":
			sawOps = true
		}
	}
	if !sawDocs || sawOps {
		t.Fatal(other["workspaces"])
	}
	serving := body(t, kc(h, "read", "--catalog", "kr://acme/docs/catalog", "--workspace", "docs", "--object", "policy/P-1")).([]any)
	if asMap(t, serving[0])["value"].(map[string]any)["v"] != float64(1) {
		t.Fatal(serving)
	}
	catalogLog := asMap(t, body(t, kc(h, "audit", "--catalog", "kr://acme/docs/catalog", "--workspace", "docs")))
	if len(catalogLog["entries"].([]any)) == 0 {
		t.Fatal(catalogLog)
	}
	expectMsg(t, kc(h, "define-workspace",
		"--catalog", "kr://missing/catalog",
		"--workspace", "x",
		"--revision", "1",
		"--source", "kr://acme/public/core=refs/heads/main",
	), "unknown catalog")
}

func TestWorkspaceAndCatalogLifecycle(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init", "--catalog", "kr://acme/catalog")
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	body(t, kc(h, "put", "--command-id", "seed", "--repo", "kr://acme/public/core", "--object", "policy/P-1", "--value", `{"v":1}`))
	body(t, kc(h, "define-workspace", "--workspace", "ops", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"))
	got := body(t, kc(h, "read", "--workspace", "ops", "--object", "policy/P-1")).([]any)
	if len(got) != 1 {
		t.Fatal(got)
	}
	body(t, kc(h, "retire-workspace", "--workspace", "ops"))
	expectCode(t, kc(h, "read", "--workspace", "ops", "--object", "policy/P-1"), "WORKSPACE_INVALID")
	expectMsg(t, kc(h, "pin-workspace", "--workspace", "ops"), "unknown command pin-workspace")
	expectCode(t, kc(h, "pin-workspace", "--workspace", "ops"), "USAGE_INVALID")
	expectCode(t, kc(h, "receipt", "--command-id", "missing"), "USAGE_INVALID")
	body(t, kc(h, "archive-catalog"))
	expectCode(t, kc(h, "define-workspace", "--workspace", "later", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"), "CATALOG_ARCHIVED")
}

func TestCatalogIsolationDoesNotShareAllow(t *testing.T) {
	h := testkit.TempDir(t)
	pub := "kr://acme/public/physical"
	secret := "kr://acme/restricted/classif"
	iso := "kr://acme/restricted/catalog"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	kc(h, "catalog-add", "--catalog", iso)
	kc(h, "repo-add", "--repo", pub)
	kc(h, "repo-add", "--repo", secret)
	body(t, kc(h, "register", "--catalog", iso, "--repo", secret))
	body(t, kc(h, "put", "--command-id", "pub-1", "--repo", pub, "--object", "Table:orders", "--value", `{"src":"public"}`))
	body(t, kc(h, "put", "--command-id", "sec-1", "--repo", secret, "--object", "Table:orders", "--value", `{"src":"secret"}`))
	body(t, kc(h, "define-workspace", "--workspace", "company", "--revision", "1", "--source", pub+"=refs/heads/main"))
	body(t, kc(h, "define-workspace", "--catalog", iso, "--workspace", "classif", "--revision", "1", "--source", secret+"=refs/heads/main"))

	body(t, kc(h, "allow", "--principal", "crew-bot", "--cmd", "read", "--repo", pub))
	body(t, kc(h, "allow", "--principal", "crew-bot", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--workspace", "company"))
	body(t, kc(h, "allow", "--principal", "classif-bot", "--cmd", "read", "--repo", secret))
	body(t, kc(h, "allow", "--principal", "classif-bot", "--cmd", "read-workspace", "--catalog", iso, "--workspace", "classif"))
	body(t, kc(h, "allow", "--principal", "classif-bot", "--action", "catalog.read", "--catalog", iso))

	crew := body(t, kc(h, "read", "--as", "crew-bot", "--workspace", "company", "--object", "Table:orders")).([]any)
	if len(crew) != 1 || asMap(t, crew[0])["repository"] != pub {
		t.Fatalf("%#v", crew)
	}
	expectCode(t, kc(h, "read", "--as", "crew-bot", "--catalog", iso, "--workspace", "classif", "--object", "Table:orders"), "FORBIDDEN")
	expectCode(t, kc(h, "read", "--as", "classif-bot", "--workspace", "company", "--object", "Table:orders"), "FORBIDDEN")
	classif := body(t, kc(h, "read", "--as", "classif-bot", "--catalog", iso, "--workspace", "classif", "--object", "Table:orders")).([]any)
	if len(classif) != 1 || asMap(t, classif[0])["repository"] != secret {
		t.Fatalf("%#v", classif)
	}
}

func TestForkPublishDoesNotCopyPersonal(t *testing.T) {
	h := testkit.TempDir(t)
	pub := "kr://acme/public/semantic"
	alice := "kr://acme/personals/alice"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	kc(h, "repo-add", "--repo", pub)
	kc(h, "repo-add", "--repo", alice)
	draft := asMap(t, asMap(t, body(t, kc(h, "put",
		"--command-id", "alice-draft",
		"--repo", alice,
		"--object", "drafts/metric-x",
		"--value", `{"text":"alice draft"}`,
	)))["result"])
	source := alice + "@" + draft["newCommit"].(string) + "/drafts/metric-x"
	body(t, kc(h, "define-workspace", "--workspace", "semantic", "--revision", "1", "--source", pub+"=refs/heads/main"))
	proposal := asMap(t, body(t, kc(h,
		"propose", "--proposal-id", "FORK-1", "--repo", pub,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/FORK-1",
		"--object", "metrics/x", "--value", `{"text":"published"}`,
		"--origin-kind", "ASSERTION",
		"--source-ref", "kc://"+strings.TrimPrefix(source, "kr://"),
	)))
	preview := asMap(t, body(t, kc(h, "preview", "--proposal", "FORK-1", "--workspace", "semantic")))
	structural := asMap(t, body(t, kc(h, "validate", "--preview", preview["previewId"].(string))))
	if structural["outcome"] != "PASSED" {
		t.Fatal(structural)
	}
	validation := asMap(t, body(t, kc(h, "record-validation", "--preview", preview["previewId"].(string), "--suite", "fork", "--outcome", "PASSED")))
	body(t, kc(h, "merge", "--proposal", proposal["proposalId"].(string), "--preview", preview["previewId"].(string), "--validation", validation["reportId"].(string)))

	live := asMap(t, body(t, kc(h, "read", "--repo", pub, "--object", "metrics/x", "--ref", "refs/heads/main")))
	if asMap(t, live["value"])["text"] != "published" {
		t.Fatal(live)
	}
	personal := asMap(t, body(t, kc(h, "read", "--repo", alice, "--object", "drafts/metric-x", "--ref", "refs/heads/main")))
	if asMap(t, personal["value"])["text"] != "alice draft" {
		t.Fatal(personal)
	}
	expectCode(t, kc(h, "read", "--repo", pub, "--object", "drafts/metric-x", "--ref", "refs/heads/main"), "KNOWLEDGE_REF_UNRESOLVED")
	prov := asMap(t, body(t, kc(h, "provenance", "--repo", pub, "--object", "metrics/x", "--ref", "refs/heads/main")))
	chain := prov["chain"].([]any)
	refs := asMap(t, chain[0])["sourceRefs"].([]any)
	if len(refs) != 1 || refs[0] != "kc://acme/personals/alice@"+draft["newCommit"].(string)+"/drafts/metric-x" {
		t.Fatal(prov)
	}
	servingNew := body(t, kc(h, "read", "--workspace", "semantic", "--object", "metrics/x")).([]any)
	if len(servingNew) != 1 {
		t.Fatal("merged fork must be visible on next read --workspace", servingNew)
	}
	servingDraft := body(t, kc(h, "read", "--workspace", "semantic", "--object", "drafts/metric-x")).([]any)
	if len(servingDraft) != 0 {
		t.Fatal("personal draft leaked into public workspace", servingDraft)
	}
}

func TestSchemaRefOnPropose(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init")
	kc(h, "repo-add", "--repo", core)
	expectCode(t, kc(h, "propose",
		"--proposal-id", "PR-schema",
		"--repo", core,
		"--target", "refs/heads/main",
		"--candidate", "refs/heads/candidates/PR-schema",
		"--object", "policy/A",
		"--value", `{"v":1}`,
		"--schema-ref", "schema/policy",
	), "SCHEMA_REVISION_UNRESOLVED")
	body(t, kc(h, "put",
		"--command-id", "schema-policy",
		"--repo", core,
		"--object", "schema/policy",
		"--value", `{"entity":"Policy","pattern":"record"}`,
	))
	proposal := asMap(t, body(t, kc(h, "propose",
		"--proposal-id", "PR-schema-ok",
		"--repo", core,
		"--target", "refs/heads/main",
		"--candidate", "refs/heads/candidates/PR-schema-ok",
		"--object", "policy/A",
		"--value", `{"v":1}`,
		"--schema-ref", "schema/policy",
	)))
	if proposal["candidateCommit"] == "" {
		t.Fatal(proposal)
	}
}

func TestWritePath(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	expectCode(t, kc(h, "put",
		"--command-id", "missing-schema",
		"--repo", "kr://acme/public/core",
		"--object", "policy/A",
		"--value", `{"v":1}`,
		"--schema-ref", "schema/policy@c1",
	), "SCHEMA_REVISION_UNRESOLVED")
	body(t, kc(h, "put",
		"--command-id", "schema-policy",
		"--repo", "kr://acme/public/core",
		"--object", "schema/policy",
		"--value", `{"entity":"Policy","pattern":"record"}`,
	))
	created := asMap(t, body(t, kc(h, "put",
		"--command-id", "create-a",
		"--repo", "kr://acme/public/core",
		"--object", "policy/A",
		"--value", `{"v":1}`,
		"--if-absent",
		"--schema-ref", "schema/policy",
		"--origin-kind", "SOURCE",
		"--actor-ref", "alice",
	)))
	if created["disposition"] != "APPLIED" {
		t.Fatal(created)
	}
	dup := kc(h, "put",
		"--command-id", "create-a-again",
		"--repo", "kr://acme/public/core",
		"--object", "policy/A",
		"--value", `{"v":2}`,
		"--if-absent",
	)
	expectCode(t, dup, "PRECONDITION_FAILED")
	draft := filepath.Join(h, "draft")
	if err := os.MkdirAll(draft, 0o755); err != nil {
		t.Fatal(err)
	}
	canonical := "---\nobject_id: runbooks/oncall\nschema_ref: schema/runbook.body\n---\n{\"text\":\"check freeze\"}\n"
	if err := os.WriteFile(filepath.Join(draft, "note.json"), []byte(canonical), 0o644); err != nil {
		t.Fatal(err)
	}
	schema := "---\nobject_id: schema/runbook.body\n---\n{\"entity\":\"Runbook\",\"pattern\":\"record\",\"fields\":{\"text\":{\"type\":\"string\",\"access\":[\"text\"]}}}\n"
	if err := os.WriteFile(filepath.Join(draft, "schema.json"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(h, "cs.json")
	preview := asMap(t, body(t, kc(h, "ingest", "--repo", "kr://acme/public/core", "--dir", draft, "--out", out)))
	files := preview["files"].([]any)
	if asMap(t, files[0])["objectId"] != "runbooks/oncall" {
		t.Fatal(preview["files"])
	}
	diagnostics := asMap(t, preview["diagnostics"])
	if diagnostics["files"] != float64(2) || diagnostics["frontmatterIdentities"] != float64(2) ||
		diagnostics["schemaObjects"] != float64(1) || diagnostics["knowledgeUnits"] != float64(1) ||
		diagnostics["explicitSchemaBindings"] != float64(1) || diagnostics["searchableBindings"] != float64(1) ||
		len(diagnostics["warnings"].([]any)) != 0 {
		t.Fatalf("ingest readiness diagnostics: %#v", diagnostics)
	}
	committed := asMap(t, body(t, kc(h, "commit", "--command-id", "ingest-1", "--changeset", out)))
	if committed["disposition"] != "APPLIED" {
		t.Fatal(committed)
	}
	receipt := asMap(t, body(t, kc(h, "receipt", "--command-id", "ingest-1")))
	if receipt["commandId"] != "ingest-1" || receipt["digest"] == "" {
		t.Fatal(receipt)
	}
	missing := kc(h, "put",
		"--command-id", "derived-bad",
		"--repo", "kr://acme/public/core",
		"--object", "derived/x",
		"--value", `{"v":1}`,
		"--origin-kind", "DERIVATION",
	)
	expectCode(t, missing, "PRECONDITION_FAILED")
	body(t, kc(h, "put",
		"--command-id", "derived-ok",
		"--repo", "kr://acme/public/core",
		"--object", "derived/x",
		"--value", `{"v":1}`,
		"--origin-kind", "DERIVATION",
		"--input-workspace-version", "vr-1",
		"--algorithm-hash", "abc",
	))
}

func TestIngestDoesNotProbeExistingSchema(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	repo := "kr://acme/public/core"
	kc(h, "repo-add", "--repo", repo)
	body(t, kc(h, "put",
		"--command-id", "schema-secret",
		"--repo", repo,
		"--object", "schema/secret",
		"--value", `{"entity":"Secret","pattern":"record","fields":{"text":{"type":"string","access":["text"]}}}`,
	))

	draft := filepath.Join(h, "untrusted-draft")
	if err := os.MkdirAll(draft, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nobject_id: notes/untrusted\nschema_ref: schema/secret\n---\n{\"text\":\"draft\"}\n"
	if err := os.WriteFile(filepath.Join(draft, "note.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	body(t, kc(h, "allow", "--principal", "untrusted-agent", "--action", "writer.preview", "--repo", repo))

	preview := asMap(t, body(t, kc(h, "ingest", "--as", "untrusted-agent", "--repo", repo, "--dir", draft)))
	diagnostics := asMap(t, preview["diagnostics"])
	if diagnostics["unverifiedBindings"] != float64(1) || diagnostics["searchableBindings"] != float64(0) {
		t.Fatalf("ingest must not inspect an existing schema: %#v", diagnostics)
	}
	warnings := diagnostics["warnings"].([]any)
	if len(warnings) != 1 || asMap(t, warnings[0])["code"] != "SCHEMA_ACCESS_UNVERIFIED" {
		t.Fatalf("existing schema access must remain explicitly unverified: %#v", diagnostics)
	}
}

func TestAuditTrail(t *testing.T) {
	h := testkit.TempDir(t)
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/public/core"), "no kc home")
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	body(t, kc(h, "put",
		"--command-id", "sync-1",
		"--repo", "kr://acme/public/core",
		"--object", "policy/P-1",
		"--value", `{"secret":true}`,
	))
	expectCode(t, kc(h, "put", "--as", "other", "--command-id", "x", "--repo", "kr://acme/public/core", "--object", "a", "--value", "1"), "FORBIDDEN")

	trail := asMap(t, body(t, kc(h, "audit", "--layer", "kc")))
	entries := trail["entries"].([]any)
	cmds := make([]string, 0, len(entries))
	var putOk, putDenied map[string]any
	for _, item := range entries {
		row := asMap(t, item)
		cmds = append(cmds, row["cmd"].(string))
		if row["cmd"] == "writer.commit" && row["status"] == "ok" {
			putOk = row
		}
		if row["cmd"] == "writer.commit" && row["status"] == "error" {
			putDenied = row
		}
	}
	joined := strings.Join(cmds, " ")
	for _, want := range []string{"local.repository.attach", "local.init", "writer.commit"} {
		if !strings.Contains(joined, want) {
			t.Fatal(cmds)
		}
	}
	if asMap(t, entries[1])["cmd"] != "local.init" || asMap(t, entries[1])["layer"] != "kc" {
		t.Fatal(entries)
	}
	if asMap(t, asMap(t, entries[1])["refs"])["catalog"] != "kr://acme/catalog" {
		t.Fatal(entries[1])
	}
	if putOk == nil || asMap(t, putOk["args"])["value"] != "<redacted>" {
		t.Fatal(putOk)
	}
	if asMap(t, putOk["refs"])["newCommit"] == "" || asMap(t, putOk["refs"])["disposition"] != "APPLIED" {
		t.Fatal(putOk)
	}
	if putDenied == nil || asMap(t, putDenied["error"])["code"] != "FORBIDDEN" {
		t.Fatal(putDenied)
	}

	inits := asMap(t, body(t, kc(h, "audit", "--layer", "kc", "--cmd", "local.init")))
	if len(inits["entries"].([]any)) != 1 {
		t.Fatal(inits)
	}

	sys := asMap(t, body(t, kc(h, "audit", "--layer", "system")))
	sawCommit, sawCatalogInit := false, false
	for _, item := range sys["entries"].([]any) {
		row := asMap(t, item)
		if row["layer"] != "system" {
			t.Fatal(row)
		}
		switch row["cmd"] {
		case "COMMIT":
			if row["face"] == "writer" && asMap(t, row["refs"])["newCommit"] != "" {
				sawCommit = true
			}
		case "init":
			if row["face"] == "catalog" {
				sawCatalogInit = true
			}
		}
	}
	if !sawCommit || !sawCatalogInit {
		t.Fatal(sys)
	}

	catalogLog := asMap(t, body(t, kc(h, "audit")))
	sawInit := false
	for _, item := range catalogLog["entries"].([]any) {
		if strings.HasPrefix(asMap(t, item)["message"].(string), "init kr://acme/catalog") {
			sawInit = true
		}
	}
	if !sawInit {
		t.Fatal(catalogLog)
	}

	again := asMap(t, body(t, kc(h, "audit")))
	for _, item := range again["entries"].([]any) {
		if asMap(t, item)["cmd"] == "audit" {
			t.Fatal("audit must not log itself", again)
		}
	}
}

// Both ways of creating a Catalog accept a bare <org>/<name> and must store the
// same kr:// id. catalog-add used to keep the raw string, so `status` and
// `--catalog` disagreed with the id init would have written.
func TestCatalogIDIsNormalizedOnEveryPath(t *testing.T) {
	h := testkit.TempDir(t)
	started := asMap(t, body(t, kc(h, "init", "--catalog", "acme/catalog")))
	if started["catalog"] != "kr://acme/catalog" {
		t.Fatalf("init did not normalize: %v", started)
	}
	added := asMap(t, body(t, kc(h, "catalog-add", "--catalog", "acme/docs")))
	if added["catalog"] != "kr://acme/docs" {
		t.Fatalf("catalog-add did not normalize: %v", added)
	}
	// The stored id is the normalized one, so the scheme-ful form addresses it
	// and re-adding either form is a duplicate.
	shown := asMap(t, body(t, kc(h, "read", "--catalog", "kr://acme/docs")))
	if shown["catalogId"] != "kr://acme/docs" {
		t.Fatalf("normalized id does not address the catalog: %v", shown)
	}
	expectMsg(t, kc(h, "catalog-add", "--catalog", "acme/docs"), "already exists")
	expectMsg(t, kc(h, "catalog-add", "--catalog", "kr://acme/docs"), "already exists")

	status := asMap(t, body(t, kc(h, "status")))
	for _, item := range status["catalogs"].([]any) {
		id := asMap(t, item)["id"].(string)
		if !strings.HasPrefix(id, "kr://") {
			t.Fatalf("unnormalized id survived to status: %v", status["catalogs"])
		}
	}
	expectMsg(t, kc(h, "catalog-add", "--catalog", "nossh"), "<org>/<name>")
}
