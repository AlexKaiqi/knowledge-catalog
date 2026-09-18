package cli

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// catalogRepositoryInventory is the consumer Catalog inventory: registered
// Repository ids plus schemaCount at published HEAD. CatalogState still stores
// ids; this assembly stays in the application layer. README is a knowledge
// object, not title/summary columns on this list.
func catalogRepositoryInventory(ws *Home, ids []string) []map[string]any {
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, describeCatalogRepository(ws, kernel.RepositoryID(id)))
	}
	return out
}

func describeCatalogRepository(ws *Home, id kernel.RepositoryID) map[string]any {
	out := map[string]any{"id": string(id)}
	if ws == nil || ws.Reader == nil {
		return out
	}
	repo, err := ws.Reader.Require(id, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		return out
	}
	commit, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		return out
	}
	if store, ok := repo.(knowledge.SchemaStore); ok {
		if schemas, schemaErr := store.SchemaObjectIDs(commit); schemaErr == nil {
			out["schemaCount"] = len(schemas)
		}
	}
	return out
}
