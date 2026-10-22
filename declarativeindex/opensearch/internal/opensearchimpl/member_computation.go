package opensearchimpl

import (
	"bytes"
	"context"
	"errors"

	"kc/kernel"
)

// ComputeMember is explicit preparation, never a query-view capability. Member
// values and vectors share the instance cache budget, nonce leases and scanner;
// neither cache contains the current caller's identity or source attribution.
func (e *openSearchEngine) ComputeMember(ctx context.Context, key kernel.Digest, compute func() ([]byte, error)) ([]byte, error) {
	if err := e.requireMutableProjection(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if key == "" || compute == nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "member computation requires identity and construction")
	}
	cache := computationCache{directory: e.computationCacheDirectory, authority: e.computationCacheAuthority, identity: "member-values-v1", memberValues: true, ctx: ctx}
	if e.namespace == "" || cache.directory == "" {
		return compute()
	}
	hits, owners, err := cache.claimEntries([]string{string(key)})
	if err != nil {
		return nil, err
	}
	if hit, ok := hits[string(key)]; ok {
		return bytes.Clone(hit.Value), nil
	}
	value, err := compute()
	if err != nil {
		return nil, errors.Join(err, cache.cancel(owners))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(owners) > 0 {
		result := computationCacheEntry{Value: bytes.Clone(value), Digest: kernel.CanonicalDigest(value)}
		if err := cache.publishEntries(owners, map[string]computationCacheEntry{string(key): result}); err != nil {
			return nil, err
		}
	}
	return value, nil
}
