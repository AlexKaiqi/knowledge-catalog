package opensearchimpl

import (
	"context"
	"errors"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
	ports "kc/discovery/projection"
	"kc/kernel"
)

func (e *openSearchEngine) computationCache(directory string) computationCache {
	if directory == "" || e.namespace == "" {
		return computationCache{}
	}
	authority := string(kernel.CanonicalDigest([]string{"embedding-instance-v2", e.base, e.namespace}))
	return computationCache{directory: filepath.Join(directory, "opensearch-computations", authority), authority: authority}
}
func (r *resourceCoordinator) RecoverComputations(ctx context.Context, directory string) (ports.ResourceRecoveryReport, error) {
	return r.client.RecoverComputations(ctx, directory)
}

// Local instance/model buckets register ownership atomically with values. The
// scan does not depend on current embedding configuration or opened engines.
func (e *openSearchEngine) RecoverComputations(ctx context.Context, directory string) (report ports.ResourceRecoveryReport, recoveryErr error) {
	if err := e.requireMutableProjection(); err != nil {
		return report, err
	}
	defer func() {
		if err := ctx.Err(); err != nil {
			recoveryErr = err
		}
	}()
	if err := ctx.Err(); err != nil {
		return report, err
	}
	cache := e.computationCache(directory)
	if cache.directory == "" {
		return report, nil
	}
	cache.existingOnly, cache.ctx = true, ctx
	var partial error
	err := cache.globalTransaction(func(models, rootMeta *bolt.Bucket) error {
		cursor := models.Cursor()
		after := append([]byte(nil), rootMeta.Get([]byte("after"))...)
		key, value := cursor.First()
		if len(after) > 0 {
			key, value = cursor.Seek(after)
			if key != nil && string(key) == string(after) {
				key, value = cursor.Next()
			}
		}
		modelKeys := [][]byte{}
		for n := 0; n < 16 && key != nil; n++ {
			if value != nil {
				return kernel.Fail(kernel.ErrPreconditionFailed, "computation model registry contains a non-model value")
			}
			modelKeys = append(modelKeys, append([]byte(nil), key...))
			key, value = cursor.Next()
		}
		last := []byte(nil)
		if key != nil {
			last = modelKeys[len(modelKeys)-1]
			report.More = true
		}
		for _, key := range modelKeys {
			if err := ctx.Err(); err != nil {
				return err
			}
			records, meta, dimensions, err := cache.partition(models.Bucket(key), rootMeta)
			if err != nil {
				report.Retained = append(report.Retained, string(key))
				partial = errors.Join(partial, err)
				continue
			}
			partition := cache
			partition.identity = string(key)
			partition.dimensions = dimensions
			partition.memberValues = dimensions == 0
			if err := partition.prune(records, meta, 0); err != nil {
				report.Retained = append(report.Retained, string(key))
				partial = errors.Join(partial, err)
				continue
			}
			remaining, _ := records.Cursor().First()
			if remaining == nil {
				if cacheBytes(meta) != 0 {
					return kernel.Fail(kernel.ErrPreconditionFailed, "empty computation partition has nonzero accounting")
				}
				if cacheBytes(rootMeta) < computationModelReservation {
					return kernel.Fail(kernel.ErrPreconditionFailed, "computation model reservation is missing")
				}
				if err := putCacheBytes(rootMeta, cacheBytes(rootMeta)-computationModelReservation); err != nil {
					return err
				}
				if err := models.DeleteBucket(key); err != nil {
					return err
				}
				report.Deleted = append(report.Deleted, string(key))
			} else {
				report.Protected = append(report.Protected, string(key))
				if len(meta.Get([]byte("after"))) > 0 {
					report.More = true
				}
			}
		}
		return rootMeta.Put([]byte("after"), last)
	})
	if err != nil {
		report.Deleted = nil
		report.Retained = append(report.Retained, cache.authority)
		return report, errors.Join(partial, err)
	}
	return report, partial
}
