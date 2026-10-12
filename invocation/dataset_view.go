package invocation

import (
	"kc/invoke"
	"kc/kernel"
	"kc/snapshot/dataset"
)

// Convert transport flags before calling Snapshot Store's accepted-history view.
func datasetManagementView(state dataset.State, id string, revision int, flags map[string]invoke.FlagValue) (map[string]any, error) {
	limit, err := PageLimit(flags, DefaultHistoryLimit, MaxHistoryPageSize)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "%s", err)
	}
	return dataset.ManagementView(state, id, revision, dataset.ManagementOptions{
		Limit: limit, Continuation: invoke.FlagString(flags, "continuation"),
	})
}
