package opensearchimpl

import (
	"context"
	"errors"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
)

// Identical embedding operations share computation, not member identity or
// source provenance. Every output slot still belongs to its original member.
func (e *openSearchEngine) embedBatchWithCache(docs []ports.CompiledDoc, guard func(func() error) error) (vectors [][][]float32, resultErr error) {
	vectors = make([][][]float32, len(docs))
	keys, texts := []string{}, map[string]string{}
	positions := make([][]string, len(docs))
	for i, doc := range docs {
		vectors[i] = make([][]float32, len(doc.Semantic))
		positions[i] = make([]string, len(doc.Semantic))
		for j, field := range doc.Semantic {
			if !e.VectorReady() || field.Model != e.vectorModel {
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "vector member %s requires registered model %s", field.Member, field.Model)
			}
			key := string(kernel.CanonicalDigest([]string{"embedding-input-v1", e.embeddingCacheIdentity, field.Model, field.Text}))
			if _, seen := texts[key]; !seen {
				keys = append(keys, key)
				texts[key] = field.Text
			}
			positions[i][j] = key
		}
	}
	if len(keys) == 0 {
		return vectors, nil
	}
	hits, owners := map[string][]float32{}, map[string]string{}
	cache := computationCache{directory: e.computationCacheDirectory, identity: e.embeddingCacheIdentity, authority: e.computationCacheAuthority, dimensions: e.vectorDimension}
	cached := guard != nil && e.namespace != "" && cache.directory != "" && cache.identity != ""
	if cached {
		if err := guard(func() error { var err error; hits, owners, err = cache.claim(keys); return completedPhysical(err) }); err != nil {
			return nil, err
		}
	}
	if cached && len(owners) > 0 {
		defer func() {
			if resultErr != nil {
				cleanupErr := guard(func() error { return completedPhysical(cache.cancel(owners)) })
				resultErr = errors.Join(resultErr, cleanupErr)
			}
		}()
	}
	misses, input := []string{}, []string{}
	for _, key := range keys {
		if _, found := hits[key]; !found {
			misses = append(misses, key)
			input = append(input, texts[key])
		}
	}
	if len(input) > 0 {
		// The provider rejects a request whose combined input exceeds its context
		// window. Each call stays inside that window; order is the request order
		// and a failed slice is not retried.
		resultVectors := make([][]float32, 0, len(input))
		reportedModel, reportedDimensions := "", 0
		for _, batch := range embeddingRequests(input) {
			result, err := e.embedder.Embed(context.Background(), retrieval.EmbeddingRequest{Texts: batch})
			if err != nil {
				return nil, err
			}
			if result.Model != "" && reportedModel != "" && result.Model != reportedModel {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "embedding response reports a different effective model")
			}
			if result.Model != "" {
				reportedModel = result.Model
			}
			if result.Dimensions != 0 && reportedDimensions != 0 && result.Dimensions != reportedDimensions {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "embedding response reports different dimensions")
			}
			if result.Dimensions != 0 {
				reportedDimensions = result.Dimensions
			}
			if len(result.Vectors) != len(batch) {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "embedding batch returned %d vectors for %d texts", len(result.Vectors), len(batch))
			}
			resultVectors = append(resultVectors, result.Vectors...)
		}
		if reportedModel != "" && reportedModel != e.vectorModel {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "embedding response reports a different effective model")
		}
		if reportedDimensions != 0 && reportedDimensions != e.vectorDimension {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "embedding response reports different dimensions")
		}
		for i, vector := range resultVectors {
			if !validEmbeddingVector(vector, e.vectorDimension) {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "embedding result is non-finite or differs from pinned dimensions")
			}
			hits[misses[i]] = append([]float32(nil), vector...)
		}
		// The configured driver identity is necessary but not enough to cache a
		// response that explicitly reports a different effective model.
		if cached && len(owners) > 0 && reportedModel == e.vectorModel {
			if err := guard(func() error { return completedPhysical(cache.publish(owners, hits)) }); err != nil {
				return nil, err
			}
		}
	}
	for i, row := range positions {
		for j, key := range row {
			vectors[i][j] = append([]float32(nil), hits[key]...)
		}
	}
	return vectors, nil
}
