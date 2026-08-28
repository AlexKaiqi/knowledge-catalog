package cli

import (
	"kc/retrieval/opensearch"
)

// PublicStores returns endpoints for status/store-ls without env secrets.
func PublicStores(file StoresFile) map[string]any {
	file = file.withDefaults()
	secrets := authoritySecretEnvs()
	secrets["opensearch"] = opensearch.EnvPassword + " or " + opensearch.EnvAPIKey
	layout := map[string]any{
		"repos": file.Layout.Repos, "catalogs": file.Layout.Catalogs,
		"projections": file.Layout.Projections, "checkouts": file.Layout.Checkouts,
	}
	if file.Layout.Catalog != "" {
		layout["catalog"] = file.Layout.Catalog
	}
	out := map[string]any{
		"layout": layout, "profile": file.Profile, "repository": file.Repository, "index": file.Index,
		"secrets": secrets,
	}
	if file.OpenSearch.URL != "" {
		openSearch := map[string]any{"url": file.OpenSearch.URL}
		if file.OpenSearch.User != "" {
			openSearch["user"] = file.OpenSearch.User
		}
		out["opensearch"] = openSearch
	}
	return out
}
