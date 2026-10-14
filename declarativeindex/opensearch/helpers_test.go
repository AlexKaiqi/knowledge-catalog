package opensearch_test

import (
	retrieval "kc/discovery"
)

func objectIDs(hits retrieval.SearchResult) []string {
	out := make([]string, len(hits.Hits))
	for i, hit := range hits.Hits {
		out[i] = string(hit.Knowledge.Address.ObjectID)
	}
	return out
}
