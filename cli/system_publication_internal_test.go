package cli

import (
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"kc/internal/repofile"
	"kc/knowledge"
)

var updateSystemEntrypoints = flag.Bool("update-system-entrypoints", false, "publish CLI registry-derived system entrypoints")

// This is both the explicit publication generator and a drift guard. Normal
// test runs never write; go generate ./cli refreshes the authored tree without
// exporting CLI internals or importing the CLI into the Knowledge protocol.
func TestSystemCLIEntrypoints(t *testing.T) {
	paths := make([]string, 0, len(cliSurface)+1)
	for path := range cliSurface {
		paths = append(paths, path)
	}
	paths = append(paths, "serve")
	sort.Strings(paths)
	groups := map[string][]any{}
	for _, path := range paths {
		surface := cliSurface[path]
		group := systemCapabilityGroup(surface.Action)
		if path == "invoke" {
			group = "object-interfaces"
		}
		if path == "serve" {
			group = "deployment"
		}
		if group == "" {
			t.Fatalf("CLI action %s requires an explicit system capability", surface.Action)
		}
		entry := helpEntries[path]
		if entry.Description == "" || entry.Usage == "" {
			t.Fatalf("CLI command %s lacks authoritative help", path)
		}
		help := Run(append([]string{"help"}, strings.Fields(path)...))
		if help.Status != 0 || !strings.Contains(help.Stdout, entry.Usage) {
			t.Fatalf("published help entry %s does not resolve: %#v", path, help)
		}
		row := map[string]any{"command": "kc " + path, "help": "kc help " + path, "description": entry.Description}
		if surface.Action != "" {
			row["action"] = surface.Action
		}
		groups[group] = append(groups[group], row)
	}
	for group, commands := range groups {
		rel := "capabilities/" + group + "/entrypoints.yaml"
		address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: knowledge.ObjectID("kc/capability/" + group), AspectName: "entrypoints"}
		raw, err := repofile.Serialize(address, rel, "schema/system/capability-entrypoints/v1", nil, map[string]any{"commands": commands})
		if err != nil {
			t.Fatal(err)
		}
		file := filepath.Join("..", "knowledge", "system", filepath.FromSlash(rel))
		if *updateSystemEntrypoints {
			if err := os.WriteFile(file, []byte(raw), 0644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		actual, err := os.ReadFile(file)
		if err != nil || string(actual) != raw {
			t.Errorf("%s differs from CLI registry/help; run go generate ./cli (read error: %v)", rel, err)
		}
	}
}

func systemCapabilityGroup(action string) string {
	switch {
	case strings.HasPrefix(action, "identity."):
		return "identity"
	case strings.HasPrefix(action, "repositories."):
		return "repositories"
	case strings.HasPrefix(action, "inventory."), strings.HasPrefix(action, "registry."):
		return "inventory"
	case strings.HasPrefix(action, "dataset."), action == "file.read":
		return "datasets"
	case strings.HasPrefix(action, "admin.grants."):
		return "permissions"
	case strings.HasPrefix(action, "knowledge.schema."):
		return "schemas"
	case action == "knowledge.read", strings.HasPrefix(action, "knowledge.history."), action == "knowledge.provenance":
		return "read-knowledge"
	case action == "knowledge.search":
		return "search-knowledge"
	case action == "knowledge.relations", action == "knowledge.traverse":
		return "explore-relations"
	case action == "resource.access", strings.HasPrefix(action, "knowledge.binding."):
		return "resource-access"
	case strings.HasPrefix(action, "writer."):
		return "publish-knowledge"
	case strings.HasPrefix(action, "governance."):
		return "governance"
	case strings.HasPrefix(action, "projection."), action == "knowledge.access.describe":
		return "projections"
	case strings.HasPrefix(action, "operations.hooks."), strings.HasPrefix(action, "operations.gates."):
		return "extensions"
	case action == "audit.read", action == "feedback.write":
		return "evidence"
	case strings.HasPrefix(action, "deployment."):
		return "deployment"
	}
	return ""
}
