package home

import (
	readcache "kc/object/cache"
)

// HydrationCacheConfig is owned by home/config; this file keeps only the
// process-local wiring that turns the declaration into a cache.

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
	return nil
}
