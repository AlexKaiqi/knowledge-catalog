package arch_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotStoreFileModuleHasIndependentOwners(t *testing.T) {
	graph := loadGraph(t)
	for _, pkg := range []string{"snapshot/dataset", "snapshot/access", "snapshot/service", "snapshot/console", "snapshot/client", "snapshot/cli"} {
		if _, exists := graph[pkg]; !exists {
			t.Errorf("Snapshot Store capability %s has no implementation owner", pkg)
			continue
		}
		for dependency, trail := range graph.reachable(pkg) {
			for _, upper := range []string{"catalog", "home", "invocation", "invoke", "server", "cli", "client", "object", "discovery/service", "discovery", "discovery/projection", "controlplane"} {
				if dependency == upper || strings.HasPrefix(dependency, upper+"/") {
					t.Errorf("%s binds the file module to external KC application %s: %v", pkg, dependency, trail)
				}
			}
		}
	}
	root := moduleRoot(t)
	for _, path := range []string{"snapshot/console/webui/package.json", "snapshot/console/webui/LICENSE", "snapshot/console/webui/NOTICE"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Errorf("Snapshot Store console must own %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "webui", "package.json")); err == nil {
		t.Error("the full console is still owned outside the Snapshot Store module")
	}
}

func snapshotClientDirectionViolations(graph depGraph) []string {
	var out []string
	for _, owner := range []string{"snapshot", "snapshot/dataset", "snapshot/access", "snapshot/service", "snapshot/console"} {
		for _, client := range []string{"snapshot/client", "snapshot/cli"} {
			if path, exists := reachDenied(graph.reachable(owner), client); exists {
				out = append(out, fmt.Sprintf("%s depends on frontend %s through %v", owner, client, path))
			}
		}
	}
	if path, exists := reachDenied(graph.reachable("snapshot/client"), "snapshot/cli"); exists {
		out = append(out, fmt.Sprintf("Snapshot SDK depends on task CLI through %v", path))
	}
	return out
}

func TestSnapshotCoreDoesNotDependOnModuleClients(t *testing.T) {
	for _, violation := range snapshotClientDirectionViolations(loadGraph(t)) {
		t.Error(violation)
	}
}

func TestSnapshotModuleClientDirectionGuardDetectsIndirectDependencies(t *testing.T) {
	for _, tc := range []struct {
		name, owner, target string
		want                bool
	}{
		{"root-to-sdk", "snapshot", "snapshot/client", true},
		{"dataset-to-cli", "snapshot/dataset", "snapshot/cli", true},
		{"service-to-sdk-private", "snapshot/service", "snapshot/client/internal/request", true},
		{"console-to-cli", "snapshot/console", "snapshot/cli", true},
		{"sdk-to-cli", "snapshot/client", "snapshot/cli", true},
		{"cli-to-sdk", "snapshot/cli", "snapshot/client", false},
		{"sdk-to-dataset", "snapshot/client", "snapshot/dataset", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			graph := depGraph{tc.owner: {"bridge"}, "bridge": {tc.target}}
			violations := snapshotClientDirectionViolations(graph)
			if (len(violations) > 0) != tc.want {
				t.Fatalf("direction guard result %v, want violation=%v", violations, tc.want)
			}
		})
	}
}
