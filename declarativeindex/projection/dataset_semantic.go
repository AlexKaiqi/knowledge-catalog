package projection

import (
	"context"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	"kc/object/reader"
)

// SemanticDatasetWindow retrieves one approximate vector window from the
// same Dataset corpus; it never interleaves unrelated source rankings.
func (idx *Index) SemanticDatasetWindow(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, request retrieval.SearchRequest, vector []float32) (retrieval.SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return retrieval.SearchResult{}, err
	}
	s, eng, release, err := idx.preparedDataset(ctx, lookup, pin, datasetContinuation{})
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	defer release()
	window, ok := eng.(ports.SemanticWindowRetriever)
	if !ok {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset index has no semantic window")
	}
	physical := s.spec
	physical.Repository = s.id
	physical.Commit = s.basis
	page, err := window.SemanticWindowContext(ctx, ports.RetrieveRequest{Spec: physical, Search: request}, vector)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	page, err = s.filterCandidates(page)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	result := retrieval.SearchResult{SearchView: retrieval.SearchView{Snapshots: pin.Repositories}, Hits: []retrieval.ObjectHit{}, Completeness: retrieval.CompletenessPartial, Dataset: &retrieval.DatasetBasis{ID: pin.SetID, Revision: pin.Revision}}
	if err := s.appendPage(page, request, false, &result, nil); err != nil {
		return retrieval.SearchResult{}, err
	}
	return result, nil
}
