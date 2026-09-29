package home

import "kc/index/opensearch"

// The readiness ports bind concrete provider checks at assembly (CAP-ORG-01
// B7, A-01): only the assembly root imports concrete adapters, so readiness
// consumers see bound functions, not providers.

// SearchBackendProbe reports whether the search backend named by the resolved
// stores configuration accepts its credentials without creating an index or
// projection. Drivers without a probeable backend report nil.
type SearchBackendProbe func(stores StoresFile) error

// NewSearchBackendProbe returns the probe bound to the concrete index adapter.
func NewSearchBackendProbe() SearchBackendProbe {
	return func(stores StoresFile) error {
		if NormalizeIndexDriver(stores.Index) != "opensearch" {
			return nil
		}
		return opensearch.Check(stores.OpenSearch)
	}
}
