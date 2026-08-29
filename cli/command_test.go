package cli

import (
	"strings"
	"testing"
)

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
	} {
		result := Run(argv)
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Fatalf("%v should be rejected: %s", argv, result.Stdout)
		}
	}
}

func TestGroupedHelpAndIdentity(t *testing.T) {
	help := Run([]string{"help", "consumer"})
	if help.Status != 0 {
		t.Fatal(help.Stdout)
	}
	who := Run([]string{"--home", t.TempDir(), "identity", "whoami"})
	if who.Status != 0 || !strings.Contains(who.Stdout, "owner") {
		t.Fatal(who.Stdout)
	}
}

func TestGroupedCatalogViewsUseCatalogServices(t *testing.T) {
	home := t.TempDir()
	if result := Run([]string{"--home", home, "local", "init", "--catalog", "kr://acme/catalog"}); result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	for _, path := range [][]string{
		{"catalog", "show"},
		{"catalog", "repository", "list"},
		{"catalog", "workspace", "list"},
	} {
		args := append([]string{"--home", home}, path...)
		if result := Run(args); result.Status != 0 {
			t.Fatalf("%v: %s", path, result.Stdout)
		}
	}
	if result := Run([]string{"--home", home, "catalog", "workspace", "show"}); result.Status == 0 || !strings.Contains(result.Stdout, "--workspace") {
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
