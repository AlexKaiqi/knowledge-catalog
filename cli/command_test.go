package cli

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"kc/kernel"
)

func TestCLICommandsIsTheCompleteStableSurface(t *testing.T) {
	commands := CLICommandsForTest()
	if len(commands) != len(cliSurface) {
		t.Fatalf("CLICommands returned %d commands for a %d-command surface", len(commands), len(cliSurface))
	}
	if !sort.StringsAreSorted(commands) {
		t.Fatalf("CLICommands must be stable and sorted: %v", commands)
	}
	for _, path := range commands {
		if !CLICommandForTest(path) {
			t.Errorf("CLICommands returned unknown command %q", path)
		}
	}
}

func TestProductCLIRefactorDefinesTheExactPublicSurface(t *testing.T) {
	want := []string{
		"access",
		"admission show",
		"attach",
		"binding show",
		"catalog archive",
		"catalog audit",
		"catalog list",
		"catalog use",
		"create",
		"dataset clone",
		"dataset define",
		"dataset overlay",
		"dataset retire",
		"deployment identity migrate",
		"deployment init",
		"deployment status",
		"deployment system publish",
		"detach",
		"diff",
		"governance preview create",
		"governance preview validate",
		"governance proposal create",
		"governance proposal merge",
		"governance validation record",
		"grant add",
		"grant list",
		"grant remove",
		"invoke",
		"log",
		"login",
		"logout",
		"operations access-spec describe",
		"operations audit access",
		"operations audit hitmap",
		"operations audit trace",
		"operations feedback record",
		"operations gate add",
		"operations gate list",
		"operations gate remove",
		"operations hook add",
		"operations hook list",
		"operations hook remove",
		"operations projection describe",
		"operations projection notice",
		"operations projection sync",
		"provenance",
		"read",
		"relations",
		"resolve",
		"schema describe",
		"schema list",
		"search",
		"show",
		"whoami",
		"writer commit",
		"writer head",
		"writer put",
		"writer receipt",
		"writer remove",
	}
	if got := CLICommandsForTest(); !slices.Equal(got, want) {
		t.Fatalf("public CLI surface\n got: %q\nwant: %q", got, want)
	}
}

func TestEveryPublicCLICommandHasLeafUsage(t *testing.T) {
	for _, path := range CLICommandsForTest() {
		usage, ok := leafUsage[path]
		if !ok || !strings.HasPrefix(usage, "kc "+path) {
			t.Errorf("%s missing leaf usage starting with %q", path, "kc "+path)
		}
		desc := helpDescriptions[path]
		if desc == "" {
			t.Errorf("%s missing semantic description", path)
		}
		help, err := helpFor(path)
		if err != nil {
			t.Errorf("kc help %s: %v", path, err)
			continue
		}
		if !strings.Contains(help, desc) || !strings.Contains(help, usage) {
			t.Errorf("kc help %s must include semantics and argv usage", path)
		}
	}
	if formatLeafHelp("serve") == "" || helpDescriptions["serve"] == "" {
		t.Fatal("serve must have leaf help")
	}
	search := formatLeafHelp("search")
	if !strings.HasPrefix(search, helpDescriptions["search"]) || !strings.Contains(search, "例：") {
		t.Fatalf("search leaf help must lead with semantics then how-to examples:\n%s", search)
	}
}

func TestComplexLeafUsageShowsHowToWriteOperands(t *testing.T) {
	seen := map[string]struct{}{}
	for _, path := range complexLeafOperands {
		if _, ok := seen[path]; ok {
			t.Errorf("duplicate complex leaf %s", path)
		}
		seen[path] = struct{}{}
		if path != "serve" && !CLICommandForTest(path) {
			t.Errorf("complex leaf %s is not a public command", path)
		}
		usage := leafUsage[path]
		if !strings.Contains(usage, "例：") || strings.Count(usage, "\n") < 1 {
			t.Errorf("%s must explain how to write values and include an example:\n%s", path, usage)
		}
	}
	for path, usage := range leafUsage {
		if !leafNeedsValueHelp(usage) {
			continue
		}
		if _, ok := seen[path]; !ok {
			t.Errorf("%s first line has a complex value but is not in complexLeafOperands", path)
		}
	}
	if strings.Contains(leafUsage["search"], `search "*"`) || strings.Contains(leafUsage["grant add"], `--action '*'`) {
		t.Fatal("examples must not teach * as SEARCH browse or as a grant action")
	}
}

func TestEveryPublicCLICommandHasAnInternalOperationAndSemanticAction(t *testing.T) {
	for path, surface := range cliSurface {
		if !operation(surface.Handler) {
			t.Errorf("%s targets missing operation %s", path, surface.Handler)
		}
		if !strings.Contains(surface.Action, ".") {
			t.Errorf("%s has non-semantic action %q", path, surface.Action)
		}
	}
}

func TestInternalHandlersMatchPublicCLIPaths(t *testing.T) {
	for path, surface := range cliSurface {
		want := strings.ReplaceAll(path, " ", "-")
		if strings.HasPrefix(surface.Handler, "knowledge-") {
			want = "knowledge-" + want
		}
		if surface.Handler != want {
			t.Errorf("%s: internal handler %q, want public path %q", path, surface.Handler, want)
		}
	}
}

func TestRetiredInternalOperationsAreUnknown(t *testing.T) {
	for _, name := range []string{
		"ingest", "allow", "revoke", "allowed", "index-sync", "index-notify",
		"resolve", "browse-schemas", "resource-access", "resource-invoke",
		"define-kset", "describe-index",
		"merge", "validate", "record-validation", "search", "relations", "read", "init",
		"admission-request", "admin-grant-add", "admin-grant-list", "admin-grant-remove",
		"catalog-show", "catalog-repo-list", "catalog-repo-attach", "catalog-repo-create",
		"catalog-repo-archive", "dataset-list", "dataset-show", "kset-list", "kset-show",
		"kset-define", "kset-retire", "kset-overlay",
		"catalog-repo-connect", "catalog-repo-connection-show", "catalog-repo-connection-check", "catalog-repo-connection-rotate",
	} {
		if operation(name) {
			t.Errorf("retired internal operation %s is still registered", name)
		}
	}
}

func TestRemovedCommandsAreRejected(t *testing.T) {
	for _, argv := range [][]string{
		{"list"}, {"vfs-read"}, {"vfs-list"}, {"vfs-write"},
		{"capabilities"}, {"expand-relations"}, {"watch-updates"}, {"list-tree"}, {"reconcile"}, {"connector-run"},
		{"maintenance", "object", "diff"}, {"maintenance", "workspace", "checkout"}, {"maintenance", "snapshot", "export"},
		{"maintenance", "workspace", "inspect"}, {"maintenance", "workspace", "sync"}, {"maintenance", "workspace", "status"},
		{"inspect"}, {"checkout"}, {"sync"}, {"snapshot-export"},
		{"pack"},
		{"writer", "ingest"}, {"catalog", "workspace", "list"}, {"catalog", "workspace", "resolve"},
		{"identity", "whoami"}, {"knowledge", "schema", "browse"}, {"knowledge", "binding", "resolve"},
		{"knowledge"}, {"knowledge", "search"}, {"knowledge", "read"}, {"knowledge", "resolve"},
		{"knowledge", "relations"}, {"knowledge", "provenance"}, {"knowledge", "log"},
		{"knowledge", "schema", "list"}, {"knowledge", "schema", "describe"},
		{"knowledge", "binding", "show"}, {"knowledge", "access"}, {"knowledge", "invoke"},
		{"resource", "access"}, {"operations", "projection", "notify"}, {"catalog", "repository", "list"},
		{"admission", "request"},
		{"admin", "grant", "add"},
		{"catalog", "show"},
		{"catalog", "repo", "attach"},
		{"catalog", "repo", "archive"},
		{"catalog", "repo", "connect"},
		{"catalog", "repo", "create"},
		{"catalog", "repo", "list"},
		{"catalog", "repo", "share", "add"},
		{"catalog", "repo", "connection", "show"},
		{"workspace", "list"},
		{"workspace", "show"},
		{"workspace", "define"},
		{"workspace", "pin"},
		{"workspace", "check"},
		{"workspace", "retire"},
		{"workspace", "overlay"},
		{"kset", "define"},
		{"kset", "retire"},
		{"kset", "overlay"},
		{"pin"},
		{"pin", "check"},
		{"pin", "--dataset", "agent"},
	} {
		result := Run(argv)
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Fatalf("%v should be rejected: %s", argv, result.Stdout)
		}
	}
	for _, argv := range [][]string{
		{"read", "--kset", "agent", "--object", "policy/A"},
		{"dataset", "define", "--kset", "docs", "--revision", "1", "--source", "kr://acme/core"},
	} {
		result := Run(argv)
		if result.Status == 0 || !strings.Contains(result.Stdout, "unknown flag --kset") {
			t.Fatalf("%v must reject --kset: %s", argv, result.Stdout)
		}
	}
	pinFlag := Run([]string{"read", "--pin", `{"setId":"agent"}`, "--object", "policy/A"})
	if pinFlag.Status == 0 || !strings.Contains(pinFlag.Stdout, "rejects --pin") {
		t.Fatalf("read --pin must be rejected: %s", pinFlag.Stdout)
	}
}

func TestIncompleteCommandFamiliesPrintGroupHelp(t *testing.T) {
	families := publicCommandFamilies(t)
	if len(families) == 0 {
		t.Fatal("expected public command families such as grant and catalog")
	}
	for _, family := range families {
		result := Run(strings.Fields(family))
		if result.Status != 1 {
			t.Fatalf("%s: want status 1, got %#v", family, result)
		}
		if strings.Contains(result.Stdout, `"error"`) || strings.Contains(result.Stdout, "unknown command") {
			t.Fatalf("%s still looks unknown: %s", family, result.Stdout)
		}
		help := Run(append([]string{"help"}, strings.Fields(family)...))
		if result.Stdout != help.Stdout {
			t.Fatalf("%s must reuse group help\n got: %q\nwant: %q", family, result.Stdout, help.Stdout)
		}
	}
	removed := Run([]string{"pin"})
	if removed.Status == 0 || !strings.Contains(removed.Stdout, "USAGE_INVALID") {
		t.Fatalf("pin is a removed command: %s", removed.Stdout)
	}
}

func publicCommandFamilies(t *testing.T) []string {
	t.Helper()
	seen := map[string]struct{}{}
	for _, path := range CLICommandsForTest() {
		parts := strings.Split(path, " ")
		for n := 1; n < len(parts); n++ {
			prefix := strings.Join(parts[:n], " ")
			if CLICommandForTest(prefix) {
				continue
			}
			seen[prefix] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for prefix := range seen {
		out = append(out, prefix)
	}
	sort.Strings(out)
	return out
}

func TestGroupedHelpAndIdentityRequiresServer(t *testing.T) {
	for _, topic := range []string{"", "consume", "write", "compose"} {
		args := []string{"help"}
		if topic != "" {
			args = append(args, topic)
		}
		help := Run(args)
		if help.Status != 0 || !strings.Contains(help.Stdout, "kc") {
			t.Fatalf("kc help %s: %#v", topic, help)
		}
	}
	unknown := Run([]string{"help", "governor"})
	if unknown.Status == 0 || !strings.Contains(unknown.Stdout, "consume, write, compose") {
		t.Fatalf("unknown help topic did not expose recovery choices: %#v", unknown)
	}
	who := Run([]string{"--home", t.TempDir(), "whoami"})
	if who.Status == 0 || !strings.Contains(who.Stdout, "requires KC Server") {
		t.Fatal(who.Stdout)
	}
}

func TestGroupedCatalogViewsUseCatalogServices(t *testing.T) {
	home := t.TempDir()
	if result := RunEmbeddedForTest([]string{"--home", home, "local", "init", "--catalog", "kr://acme/catalog"}, nil); result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	for _, path := range [][]string{
		{"catalog", "list"},
		{"show"},
	} {
		args := append([]string{"--home", home}, path...)
		if result := runWithTelemetryMode(args, nil, true); result.Status != 0 {
			t.Fatalf("%v: %s", path, result.Stdout)
		}
	}
	for _, argv := range [][]string{
		{"catalog", "show"},
		{"catalog", "repo", "list"},
		{"workspace", "list"},
		{"workspace", "show"},
		{"workspace", "define"},
		{"workspace", "pin"},
		{"workspace", "check"},
		{"workspace", "retire"},
		{"workspace", "overlay"},
		{"kset", "define"},
		{"kset", "retire"},
		{"kset", "overlay"},
	} {
		result := runWithTelemetryMode(append([]string{"--home", home}, argv...), nil, true)
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Fatalf("%v should be rejected: %s", argv, result.Stdout)
		}
	}
}

func TestEveryInternalOperationReferencedByCLIHasTheDeclaredStage(t *testing.T) {
	for path, surface := range cliSurface {
		cmd := commands[surface.Handler]
		if knowledgeCLIPath(path) || strings.HasPrefix(path, "writer ") || strings.HasPrefix(path, "governance ") {
			if cmd.stage != stageGoverned {
				t.Errorf("%s should be governed", path)
			}
		}
	}
}

func TestLimitFlagRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"abc", "-1", "1.5"} {
		if _, err := limitFrom(map[string]FlagValue{"limit": raw}, 50); err == nil {
			t.Errorf("--limit %s should be rejected", raw)
		}
	}
	got, err := limitFrom(map[string]FlagValue{}, 50)
	if err != nil || got != 50 {
		t.Fatalf("absent --limit: %d %v", got, err)
	}
}

func TestPageLimitTreatsZeroAsDefaultAndRejectsOversized(t *testing.T) {
	got, err := pageLimit(map[string]FlagValue{"limit": "0"}, 50, 200)
	if err != nil || got != 50 {
		t.Fatalf("limit 0: %d %v", got, err)
	}
	got, err = pageLimit(map[string]FlagValue{}, 50, 200)
	if err != nil || got != 50 {
		t.Fatalf("omitted limit: %d %v", got, err)
	}
	if _, err := pageLimit(map[string]FlagValue{"limit": "201"}, 50, 200); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("oversized page: %v", err)
	}
}

func TestAddressCoordinatesRejectMemberWithoutAspect(t *testing.T) {
	if usesAddress(map[string]FlagValue{"object": "policy/x"}) {
		t.Fatal("object-only is not an Address")
	}
	if !usesAddress(map[string]FlagValue{"object": "policy/x", "aspect": "io"}) {
		t.Fatal("aspect is an Address")
	}
	if !usesAddress(map[string]FlagValue{"object": "policy/x", "member": "user:bob"}) {
		t.Fatal("member without aspect is still an Address attempt")
	}
	if _, err := addressFrom(map[string]FlagValue{"object": "policy/x", "member": "user:bob"}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("member without aspect: %v", err)
	}
}

func TestKnowledgeHistoryCommandsRejectAddressCoordinatesOnThePublicSurface(t *testing.T) {
	for _, argv := range [][]string{
		{"--server", "http://127.0.0.1:9", "log", "--dataset", "agent", "--object", "policy/x", "--aspect", "io"},
		{"--server", "http://127.0.0.1:9", "log", "--dataset", "agent", "--object", "policy/x", "--member", "user:bob"},
		{"--server", "http://127.0.0.1:9", "provenance", "--dataset", "agent", "--object", "policy/x", "--aspect", "io"},
		{"--server", "http://127.0.0.1:9", "provenance", "--dataset", "agent", "--object", "policy/x", "--member", "user:bob"},
	} {
		result := Run(argv)
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Fatalf("%v should reject Address coordinates before contacting the server: %s", argv, result.Stdout)
		}
	}
}
