package cli

import (
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	objectmodel "kc/object"
	repofile "kc/object/format"
)

var updateSystemEntrypoints = flag.Bool("update-system-entrypoints", false, "publish CLI registry-derived system entrypoints")

// This is both the explicit publication generator and a drift guard. Normal
// test runs never write; go generate ./cli refreshes the authored tree without
// exporting CLI internals or importing the CLI into the Object protocol.
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
		address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: objectmodel.ObjectID("kc/capability/" + group), AspectName: "entrypoints"}
		raw, err := repofile.Serialize(address, rel, "schema/system/capability-entrypoints/v1", nil, map[string]any{"commands": commands})
		if err != nil {
			t.Fatal(err)
		}
		file := filepath.Join("..", "systemrepo", "content", filepath.FromSlash(rel))
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
	case strings.HasPrefix(action, "object.schema."):
		return "schemas"
	case action == "object.read", strings.HasPrefix(action, "object.history."), action == "object.provenance":
		return "read-object"
	case action == "object.search":
		return "search-object"
	case action == "object.relations", action == "object.traverse":
		return "explore-relations"
	case action == "resource.access", strings.HasPrefix(action, "object.binding."):
		return "resource-access"
	case strings.HasPrefix(action, "writer."):
		return "publish-object"
	case strings.HasPrefix(action, "projection."), action == "object.access.describe":
		return "projections"
	case strings.HasPrefix(action, "operations.hooks."):
		return "extensions"
	case action == "audit.read", action == "feedback.write":
		return "evidence"
	case strings.HasPrefix(action, "deployment."):
		return "deployment"
	}
	return ""
}
