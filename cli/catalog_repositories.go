package cli

import (
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

const (
	repositoryProfilePresent     = "present"
	repositoryProfileMissing     = "missing"
	repositoryProfileUnsupported = "unsupported"
)

// catalogRepositoryInventory is the consumer Catalog inventory: each registered
// Repository id plus the reserved source-profile object at published HEAD.
// CatalogState still stores ids; this assembly stays in the application layer.
func catalogRepositoryInventory(ws *Home, ids []string) []map[string]any {
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, describeCatalogRepository(ws, kernel.RepositoryID(id)))
	}
	return out
}

func describeCatalogRepository(ws *Home, id kernel.RepositoryID) map[string]any {
	out := map[string]any{"id": string(id), "profile": repositoryProfileUnsupported}
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
	out["profile"] = repositoryProfileMissing
	if store, ok := repo.(knowledge.SchemaStore); ok {
		if schemas, schemaErr := store.SchemaObjectIDs(commit); schemaErr == nil {
			out["schemaCount"] = len(schemas)
		}
	}
	resolution, err := repo.Resolve(knowledge.SourceProfileObjectID, commit)
	if err != nil || resolution.Status != knowledge.StatusResolved {
		return out
	}
	value, err := repo.Read(knowledge.SourceProfileObjectID, commit)
	if err != nil {
		return out
	}
	title, summary, ok := sourceProfileText(value.Value)
	if !ok {
		return out
	}
	out["profile"] = repositoryProfilePresent
	out["title"] = title
	out["summary"] = summary
	return out
}

func sourceProfileText(value any) (title, summary string, ok bool) {
	body, ok := value.(map[string]any)
	if !ok {
		return "", "", false
	}
	title, _ = body["title"].(string)
	summary, _ = body["summary"].(string)
	title = strings.TrimSpace(title)
	summary = strings.TrimSpace(summary)
	if title == "" || summary == "" {
		return "", "", false
	}
	return title, summary, true
}
