package home

import (
	"context"

	"kc/kernel"
	"kc/knowledge"
	readcache "kc/retrieval/cache"
)

// HydrationCacheConfig controls process-local Snapshot body acceleration.
// An omitted configuration enables bounded caching and hot-object warmup.
// Cold-start identity paging is enabled explicitly with ColdStart.
type HydrationCacheConfig struct {
	Disabled      bool  `json:"disabled,omitempty" yaml:"disabled,omitempty"`
	MaxBytes      int64 `json:"maxBytes,omitempty" yaml:"maxBytes,omitempty"`
	MaxEntries    int   `json:"maxEntries,omitempty" yaml:"maxEntries,omitempty"`
	WarmLimit     int   `json:"warmLimit,omitempty" yaml:"warmLimit,omitempty"`
	WarmBatchSize int   `json:"warmBatchSize,omitempty" yaml:"warmBatchSize,omitempty"`
	ColdStart     *bool `json:"coldStart,omitempty" yaml:"coldStart,omitempty"`
}

func (ws *Home) configureHydration() error {
	cfg := readcache.Config{}
	if settings := ws.Stores.HydrationCache; settings != nil {
		if settings.Disabled {
			return nil
		}
		cfg.MaxBytes, cfg.MaxEntries = settings.MaxBytes, settings.MaxEntries
		cfg.WarmLimit, cfg.WarmBatchSize = settings.WarmLimit, settings.WarmBatchSize
		if settings.ColdStart != nil {
			cfg.ColdStart = *settings.ColdStart
		}
	}
	cache, err := readcache.New(cfg)
	if err != nil {
		return err
	}
	ws.ReadCache, ws.Hydrator = cache, cache
	ws.Reader.SetHydrator(cache)
	ws.Index.SetHydrator(cache)
	return nil
}

type cacheWarmer struct{ cache *readcache.Cache }

func (cacheWarmer) ID() string { return "snapshot-body-cache/v1" }

func (w cacheWarmer) Reconcile(ctx context.Context, repo knowledge.Repository, commit kernel.CommitID) error {
	return w.cache.Warm(ctx, repo, commit)
}
