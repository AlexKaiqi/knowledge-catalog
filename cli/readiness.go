package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kc/controlplane"
	"kc/retrieval/opensearch"
	"kc/snapshot"
	"kc/snapshot/commandlog"
)

type readinessResult struct {
	Status     string `json:"status"`
	Surface    string `json:"surface"`
	ReasonCode string `json:"reasonCode,omitempty"`
}

type readinessCacheEntry struct {
	fingerprint string
	expires     time.Time
	result      readinessResult
}

type readinessCache struct {
	home     string
	ttl      time.Duration
	mu       sync.Mutex
	entries  map[string]readinessCacheEntry
	inflight map[string]chan struct{}
}

func newReadinessCache(home string, ttl time.Duration) *readinessCache {
	return &readinessCache{home: home, ttl: ttl, entries: map[string]readinessCacheEntry{}, inflight: map[string]chan struct{}{}}
}

func (c *readinessCache) surface(surface string) readinessResult {
	if !knownReadinessSurface(surface) {
		return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "SURFACE_UNKNOWN"}
	}
	for {
		fingerprint := readinessFingerprint(c.home, surface)
		c.mu.Lock()
		if cached, ok := c.entries[surface]; ok && cached.fingerprint == fingerprint && time.Now().Before(cached.expires) {
			c.mu.Unlock()
			return cached.result
		}
		if done := c.inflight[surface]; done != nil {
			c.mu.Unlock()
			<-done
			continue
		}
		done := make(chan struct{})
		c.inflight[surface] = done
		c.mu.Unlock()

		result := readiness(c.home, surface)
		c.mu.Lock()
		c.entries[surface] = readinessCacheEntry{fingerprint: fingerprint, expires: time.Now().Add(c.ttl), result: result}
		delete(c.inflight, surface)
		close(done)
		c.mu.Unlock()
		return result
	}
}

func knownReadinessSurface(surface string) bool {
	return surface == "consumer" || surface == "writer" || surface == "search"
}

func (c *readinessCache) overall() readinessResult {
	for _, surface := range []string{"consumer", "writer", "search"} {
		if result := c.surface(surface); result.Status != "ready" {
			result.Surface = "all"
			return result
		}
	}
	return readinessResult{Status: "ready", Surface: "all"}
}

// readinessFingerprint invalidates the short probe cache immediately when
// the local home/config/evidence target changes. Remote-only changes are
// observed at the TTL boundary, keeping expensive backend checks off every
// load-balancer poll.
func readinessFingerprint(home, surface string) string {
	paths := []string{home, layoutPath(home), storesPath(home)}
	if surface == "writer" {
		paths = append(paths,
			filepath.Join(home, "access.jsonl"),
			filepath.Join(home, "writer.json"),
			filepath.Join(home, "control.json"),
		)
	}
	var out strings.Builder
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(&out, "%s:%v;", path, err)
			continue
		}
		fmt.Fprintf(&out, "%s:%s:%d:%d;", path, info.Mode(), info.Size(), info.ModTime().UnixNano())
	}
	return out.String()
}

func readiness(home, surface string) readinessResult {
	if !knownReadinessSurface(surface) {
		return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "SURFACE_UNKNOWN"}
	}
	if !homeReady(home) {
		return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "HOME_NOT_INITIALIZED"}
	}
	stores, err := ReadStores(home)
	if err != nil || stores.validateProfile() != nil {
		return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "HOME_CONFIGURATION_INVALID"}
	}
	// Validate local Catalog state without opening every attached Repository:
	// one route's remote outage must not make unrelated routes globally unready.
	file, err := ReadHome(home)
	if err != nil {
		return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "HOME_DISCOVERY_FAILED"}
	}
	if _, _, err := openCatalogs(home, file, snapshot.NewRegistry()); err != nil {
		return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "CATALOG_STATE_UNAVAILABLE"}
	}
	if surface == "search" && normalizeIndexDriver(stores.Index) == "opensearch" {
		if err := opensearch.Check(stores.OpenSearch); err != nil {
			return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "SEARCH_BACKEND_UNAVAILABLE"}
		}
	}
	if surface == "writer" {
		if _, err := commandlog.New(commandlog.NewFileStore(filepath.Join(home, "writer.json"))); err != nil {
			return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "COMMAND_LOG_UNAVAILABLE"}
		}
		if _, err := controlplane.NewFileControlState(filepath.Join(home, "control.json")).LoadBundle(); err != nil {
			return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "CONTROL_STATE_UNAVAILABLE"}
		}
		if err := evidenceWriteProbe(home); err != nil {
			return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "EVIDENCE_STORE_UNWRITABLE"}
		}
	}
	return readinessResult{Status: "ready", Surface: surface}
}

func overallReadiness(home string) readinessResult {
	for _, surface := range []string{"consumer", "writer", "search"} {
		if result := readiness(home, surface); result.Status != "ready" {
			result.Surface = "all"
			return result
		}
	}
	return readinessResult{Status: "ready", Surface: "all"}
}

// evidenceWriteProbe verifies the real access target when it exists. For a new
// target it exercises the same directory durability primitive without
// appending a fake audit/access event or changing catalog state. HTTP probes
// cache this result briefly, so fsync is not repeated for every poll.
func evidenceWriteProbe(home string) error {
	accessPath := filepath.Join(home, "access.jsonl")
	if info, err := os.Stat(accessPath); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("access evidence target is not a regular file")
		}
		file, err := os.OpenFile(accessPath, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return err
		}
		return file.Close()
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(home, ".kc-ready-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write([]byte("ready\n")); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
