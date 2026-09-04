package cli

import (
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

func TestRemovedCommandsAreRejected(t *testing.T) {
	for _, argv := range [][]string{
		{"read"}, {"search"}, {"list"}, {"vfs-read"}, {"vfs-list"}, {"vfs-write"},
		{"capabilities"}, {"expand-relations"}, {"watch-updates"}, {"list-tree"}, {"reconcile"}, {"connector-run"},
		{"maintenance", "object", "diff"}, {"maintenance", "workspace", "checkout"}, {"maintenance", "snapshot", "export"},
		{"maintenance", "workspace", "inspect"}, {"maintenance", "workspace", "sync"}, {"maintenance", "workspace", "status"},
		{"inspect"}, {"checkout"}, {"diff"}, {"sync"}, {"snapshot-export"},
	} {
		result := Run(argv)
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Fatalf("%v should be rejected: %s", argv, result.Stdout)
		}
	}
}

func TestGroupedHelpAndIdentityRequiresServer(t *testing.T) {
	for _, topic := range []string{"", "consumer", "provider", "governor"} {
		args := []string{"help"}
		if topic != "" {
			args = append(args, topic)
		}
		help := Run(args)
		if help.Status != 0 || !strings.Contains(help.Stdout, "kc") {
			t.Fatalf("kc help %s: %#v", topic, help)
		}
	}
	unknown := Run([]string{"help", "unknown"})
	if unknown.Status == 0 || !strings.Contains(unknown.Stdout, "consumer, provider, or governor") {
		t.Fatalf("unknown help topic did not expose recovery choices: %#v", unknown)
	}
	who := Run([]string{"--home", t.TempDir(), "whoami"})
	if who.Status == 0 || !strings.Contains(who.Stdout, "requires KC Server") {
		t.Fatal(who.Stdout)
	}
}

func TestGroupedCatalogViewsUseCatalogServices(t *testing.T) {
	home := t.TempDir()
	if result := Run([]string{"--home", home, "local", "init", "--catalog", "kr://acme/catalog"}); result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	for _, path := range [][]string{
		{"catalog", "list"},
		{"catalog", "show"},
		{"catalog", "repo", "list"},
		{"catalog", "workspace", "list"},
	} {
		args := append([]string{"--home", home}, path...)
		if result := runWithTelemetryMode(args, nil, true); result.Status != 0 {
			t.Fatalf("%v: %s", path, result.Stdout)
		}
	}
	if result := runWithTelemetryMode([]string{"--home", home, "catalog", "workspace", "show"}, nil, true); result.Status == 0 || !strings.Contains(result.Stdout, "--workspace") {
		t.Fatal(result.Stdout)
	}
}

func TestEveryInternalOperationReferencedByCLIHasTheDeclaredStage(t *testing.T) {
	for path, surface := range cliSurface {
		cmd := commands[surface.Handler]
		if strings.HasPrefix(path, "knowledge ") || strings.HasPrefix(path, "writer ") || strings.HasPrefix(path, "governance ") {
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
		{"--server", "http://127.0.0.1:9", "knowledge", "log", "--workspace", "agent", "--object", "policy/x", "--aspect", "io"},
		{"--server", "http://127.0.0.1:9", "knowledge", "log", "--workspace", "agent", "--object", "policy/x", "--member", "user:bob"},
		{"--server", "http://127.0.0.1:9", "knowledge", "provenance", "--workspace", "agent", "--object", "policy/x", "--aspect", "io"},
		{"--server", "http://127.0.0.1:9", "knowledge", "provenance", "--workspace", "agent", "--object", "policy/x", "--member", "user:bob"},
	} {
		result := Run(argv)
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Fatalf("%v should reject Address coordinates before contacting the server: %s", argv, result.Stdout)
		}
	}
}
