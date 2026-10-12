package service

import (
	"context"
	"net/http"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/dataset"
)

// repositoryList lists the service's registered source IDs. Its
// inventory.read admission does not confer source-file read or publication rights.
func (h *handler) repositoryList(w http.ResponseWriter, r *http.Request) {
	h.withResources(w, r, "repository-list", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		cat, err := openAuthorizedDatasets(ctx, caller, resources, "", "inventory.read")
		if err != nil {
			return nil, err
		}
		state := dataset.NormalizeState(cat.DumpState())
		rows := make([]map[string]any, 0, len(state.Repositories))
		for _, id := range state.Repositories {
			row := map[string]any{"id": id}
			if resources.DescribeRepository != nil {
				details, err := resources.DescribeRepository(ctx, caller, kernel.RepositoryID(id))
				if err != nil {
					return nil, err
				}
				for key, value := range details {
					if key != "id" {
						row[key] = value
					}
				}
			}
			rows = append(rows, row)
		}
		return map[string]any{"repositories": rows}, nil
	})
}
