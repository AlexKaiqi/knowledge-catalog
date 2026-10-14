package cli

import (
	"kc/invoke"
	"strings"
	"testing"
)

func TestRetiredCatalogSelectionIsRejected(t *testing.T) {
	for _, argv := range [][]string{{"show", "--catalog", "old"}, {"catalog", "list"}, {"catalog", "use", "old"}, {"catalog", "archive"}} {
		result := Run(argv)
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Fatalf("retired input %v = %#v", argv, result)
		}
	}
}

func TestSearchInheritsConfiguredDatasetWithinSingleService(t *testing.T) {
	t.Setenv("KC_DATASET", "configured")
	flags := map[string]FlagValue{}
	bindRemoteEnvironment("search", flags)
	if invoke.FlagString(flags, "dataset") != "configured" {
		t.Fatalf("service inventory replaced the configured Dataset: %#v", flags)
	}
}
