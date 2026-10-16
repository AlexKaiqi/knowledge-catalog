package capability_test

import (
	"slices"
	"strings"
	"testing"

	"kc/declarativeindex/capability"
	indexcli "kc/declarativeindex/cli"
)

func TestAffordanceLinksExistingMaintenanceCommands(t *testing.T) {
	affordance := capability.Affordance()
	if affordance.Name != "declarative-index" || !strings.Contains(affordance.Usage, "同一依据") || !strings.Contains(affordance.Usage, "不现场建索引") {
		t.Fatalf("plugin guarantee/navigation missing: %#v", affordance)
	}
	for _, entry := range indexcli.Commands() {
		if !slices.Contains(affordance.Tasks, entry.Path) {
			t.Fatalf("plugin description omits maintenance task %s", entry.Path)
		}
	}
}
