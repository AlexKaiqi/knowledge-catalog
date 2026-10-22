package home

import (
	"kc/declarativeindex/opensearch"
	index "kc/discovery/projection"
)

// Instance query and resource-maintenance clients use one namespace contract.
func instanceStores(stores StoresFile, namespace string) StoresFile {
	if namespace != "" {
		if stores.OpenSearch.Namespace != "" {
			namespace = stores.OpenSearch.Namespace + "/" + namespace
		}
		stores.OpenSearch.Namespace = namespace
	}
	return stores
}

func indexResourceCoordinator(file HomeFile, stores StoresFile) (index.ResourceCoordinator, error) {
	if selectedIndexDriver(file, stores) != "opensearch" {
		return nil, nil
	}
	return opensearch.OpenResources(stores.OpenSearch)
}
