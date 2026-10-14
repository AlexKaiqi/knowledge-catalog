package invocation

import (
	"testing"

	"kc/invoke"
	"kc/kernel"
	"kc/snapshot/dataset"
)

func TestDatasetManagementViewConvertsHistoryFlags(t *testing.T) {
	state := dataset.State{Datasets: []dataset.Dataset{{SetID: "files", Revision: 501}}}
	for revision := 1; revision <= 501; revision += 2 {
		state.DatasetVersions = append(state.DatasetVersions, dataset.Dataset{SetID: "files", Revision: revision})
	}
	for _, limit := range []invoke.FlagValue{"", "0", 0} {
		view, err := datasetManagementView(state, "files", 1, map[string]invoke.FlagValue{"limit": limit})
		if err != nil || len(view["versions"].([]int)) != DefaultHistoryLimit || view["revision"] != 1 {
			t.Fatalf("default limit %v changed history or point lookup: %#v, %v", limit, view, err)
		}
	}
	view, err := datasetManagementView(state, "files", 0, map[string]invoke.FlagValue{"limit": MaxHistoryPageSize})
	if err != nil || len(view["versions"].([]int)) != MaxHistoryPageSize {
		t.Fatalf("maximum history page changed: %#v, %v", view, err)
	}
	if _, err := datasetManagementView(state, "files", 0, map[string]invoke.FlagValue{"continuation": view["continuation"]}); err != nil {
		t.Fatalf("transport lost the core continuation: %v", err)
	}
	for _, limit := range []string{"-1", "nope", "201"} {
		_, err := datasetManagementView(state, "files", 0, map[string]invoke.FlagValue{"limit": limit})
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("history limit %q: %v", limit, err)
		}
	}
}
