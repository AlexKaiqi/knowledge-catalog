package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func commandSource(t *testing.T, path, source string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	return fset, file
}

func TestInventoryReadsExplicitModuleCompositionWithSourceLocations(t *testing.T) {
	fset, file := commandSource(t, "cli/surface.go", `package fixture
import snapshotcli "kc/snapshot/cli"
var cliSurface = composeCLICommands(snapshotcli.Commands(), map[string]commandSurface{
  "read": {"object-read", "object.read"},
})`)
	root, err := cliDeclarations(fset, file, "cli/surface.go")
	if err != nil || !usesSnapshotCommands(file) {
		t.Fatalf("explicit composition: %v", err)
	}
	fset, file = commandSource(t, "snapshot/cli/commands.go", `package fixture
var commandEntries = []Command{
  {Path: "dataset show", Handler: "dataset-show", Action: "inventory.read", Usage: "sample fake command"},
}`)
	module, err := snapshotCommandDeclarations(fset, file, "snapshot/cli/commands.go")
	if err != nil {
		t.Fatal(err)
	}
	entries := append(root, module...)
	if err := validateCommandInventory(entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0] != (commandEntry{"dataset show", "inventory.read", "snapshot/cli/commands.go", 3}) || entries[1] != (commandEntry{"read", "object.read", "cli/surface.go", 4}) {
		t.Fatalf("composition omitted a module or counted help examples: %#v", entries)
	}
}

func TestInventoryReadsBothOwnedCLIContributions(t *testing.T) {
	fset, file := commandSource(t, "cli/surface.go", `package fixture
import snapshotcli "kc/snapshot/cli"
import objectcli "kc/object/cli"
var cliSurface = composeObjectCLICommands(objectcli.Commands(), composeCLICommands(snapshotcli.Commands(), map[string]commandSurface{
  "search": {"object-search", "object.search"},
}))`)
	entries, err := cliDeclarations(fset, file, "cli/surface.go")
	if err != nil || len(entries) != 1 || entries[0].Command != "search" || !usesSnapshotCommands(file) {
		t.Fatalf("nested owned contributions lost the host declaration: %#v %v", entries, err)
	}
	for _, expression := range []string{
		`composeObjectCLICommands(other.Commands(), composeCLICommands(snapshotcli.Commands(), map[string]commandSurface{}))`,
		`composeObjectCLICommands(objectcli.Commands(config), composeCLICommands(snapshotcli.Commands(), map[string]commandSurface{}))`,
		`composeObjectCLICommands(objectcli.Commands(), dynamicHost())`,
	} {
		_, bad := commandSource(t, "cli/surface.go", "package fixture\nimport snapshotcli \"kc/snapshot/cli\"\nimport objectcli \"kc/object/cli\"\nvar cliSurface = "+expression)
		if _, err := cliDeclarations(fset, bad, "cli/surface.go"); err == nil {
			t.Fatal("dynamic or foreign Object contribution escaped the closed inventory")
		}
	}
}

func TestInventoryRejectsUnknownOrDynamicCommandComposition(t *testing.T) {
	for name, declaration := range map[string]string{
		"unknownAssembler": `other(snapshotcli.Commands(), map[string]commandSurface{})`,
		"foreignModule":    `composeCLICommands(other.Commands(), map[string]commandSurface{})`,
		"missingModule":    `composeCLICommands(map[string]commandSurface{})`,
		"dynamicKC":        `composeCLICommands(snapshotcli.Commands(), currentCommands())`,
		"dynamicModule":    `composeCLICommands(snapshotcli.Commands(config), map[string]commandSurface{})`,
	} {
		t.Run(name, func(t *testing.T) {
			fset, file := commandSource(t, "cli/surface.go", "package fixture\nimport snapshotcli \"kc/snapshot/cli\"\nvar cliSurface = "+declaration)
			if _, err := cliDeclarations(fset, file, "cli/surface.go"); err == nil {
				t.Fatal("dynamic or unrecognized module composition silently changed the command denominator")
			}
		})
	}
}

func TestInventoryReadsFourOwnedCLIContributionsAndRejectsForeignOrRepeatedOwners(t *testing.T) {
	imports := `package fixture
import snapshotcli "kc/snapshot/cli"
import objectcli "kc/object/cli"
import discoverycli "kc/discovery/cli"
import declarativeindexcli "kc/declarativeindex/cli"
var cliSurface = `
	expression := `composeDeclarativeIndexCLICommands(declarativeindexcli.Commands(), composeDiscoveryCLICommands(discoverycli.Commands(), composeObjectCLICommands(objectcli.Commands(), composeCLICommands(snapshotcli.Commands(), map[string]commandSurface{"create":{"create","repositories.create"}}))))`
	fset, file := commandSource(t, "cli/surface.go", imports+expression)
	entries, err := cliDeclarations(fset, file, "cli/surface.go")
	expr, _ := namedValue(file, "cliSurface")
	_, modules, compositionErr := commandComposition(file, expr)
	if err != nil || compositionErr != nil || len(entries) != 1 || entries[0].Command != "create" || len(modules) != 4 {
		t.Fatalf("owned denominator lost module contributions: %#v %#v %v %v", entries, modules, err, compositionErr)
	}
	for _, bad := range []string{
		strings.Replace(expression, "discoverycli.Commands()", "objectcli.Commands()", 1),
		strings.Replace(expression, "declarativeindexcli.Commands()", "declarativeindexcli.Commands(config)", 1),
		`composeDiscoveryCLICommands(discoverycli.Commands(),composeDiscoveryCLICommands(discoverycli.Commands(),map[string]commandSurface{}))`,
		`composeDeclarativeIndexCLICommands(declarativeindexcli.Commands(),dynamicHost())`,
		`composeUnownedCLICommands(discoverycli.Commands(),map[string]commandSurface{})`,
	} {
		fset, file := commandSource(t, "cli/surface.go", imports+bad)
		if _, err := cliDeclarations(fset, file, "cli/surface.go"); err == nil {
			t.Fatal("foreign, dynamic or repeated module changed the closed command inventory")
		}
	}
}

func TestInventoryRejectsInvalidModuleDeclarationsAndDuplicateOwners(t *testing.T) {
	for name, declaration := range map[string]string{
		"dynamicList":    `currentCommands()`,
		"dynamicPath":    `[]Command{{Path: currentPath, Action: "file.read"}}`,
		"dynamicAction":  `[]Command{{Path: "files read", Action: currentAction}}`,
		"missingAction":  `[]Command{{Path: "files read"}}`,
		"positional":     `[]Command{{"files read", "file.read"}}`,
		"duplicateField": `[]Command{{Path: "files read", Path: "files list", Action: "file.read"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			fset, file := commandSource(t, "commands.go", "package fixture\nvar commandEntries = "+declaration)
			if _, err := snapshotCommandDeclarations(fset, file, "commands.go"); err == nil {
				t.Fatal("incomplete module declarations were silently omitted")
			}
		})
	}
	err := validateCommandInventory([]commandEntry{{"read", "object.read", "cli/surface.go", 4}, {"read", "file.read", "snapshot/cli/commands.go", 3}})
	if err == nil || !strings.Contains(err.Error(), "cli/surface.go:4") || !strings.Contains(err.Error(), "snapshot/cli/commands.go:3") {
		t.Fatalf("duplicate command must identify both owners: %v", err)
	}
}

func TestInventoryIncludesOwnedLiveAccessCommandsAndRejectsForeignOrRepeatedContribution(t *testing.T) {
	imports := `package fixture
import accesscli "kc/liveaccess/cli"
var cliSurface = `
	expression := `composeLiveAccessCLICommands(accesscli.Commands(),map[string]commandSurface{"read":{"object-read","object.read"}})`
	fset, file := commandSource(t, "cli/surface.go", imports+expression)
	expr, _ := namedValue(file, "cliSurface")
	_, modules, err := commandComposition(file, expr)
	entries, declarationsErr := cliDeclarations(fset, file, "cli/surface.go")
	if err != nil || declarationsErr != nil || !modules["liveaccess/cli"] || len(entries) != 1 || entries[0].Command != "read" {
		t.Fatalf("owned LiveAccess contribution lost the command denominator: %#v %#v %v %v", modules, entries, err, declarationsErr)
	}
	for _, bad := range []string{
		strings.Replace(expression, "accesscli.Commands()", "other.Commands()", 1),
		strings.Replace(expression, "accesscli.Commands()", "accesscli.Commands(config)", 1),
		`composeLiveAccessCLICommands(accesscli.Commands(),composeLiveAccessCLICommands(accesscli.Commands(),map[string]commandSurface{}))`,
		`composeLiveAccessCLICommands(accesscli.Commands(),dynamicHost())`,
	} {
		fset, file := commandSource(t, "cli/surface.go", imports+bad)
		if _, err := cliDeclarations(fset, file, "cli/surface.go"); err == nil {
			t.Fatal("foreign, repeated or dynamic LiveAccess changed the closed command inventory")
		}
	}
}
