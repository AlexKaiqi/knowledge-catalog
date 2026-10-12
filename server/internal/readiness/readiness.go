package readiness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kc/controlplane"
	apphome "kc/home"
	"kc/snapshot"
)

type Result struct {
	Status     string `json:"status"`
	Surface    string `json:"surface"`
	ReasonCode string `json:"reasonCode,omitempty"`
}

type cacheEntry struct {
	fingerprint string
	expires     time.Time
	result      Result
}

// Providers binds the assembly-root checks: the concrete index adapter is
// wired by home (CAP-ORG-01 B7, A-01), so this package sees ports, not
// providers.
type Providers struct {
	// SearchBackend reports whether the configured search backend accepts the
	// connection; it reports nil for drivers with nothing to probe.
	SearchBackend func(stores apphome.StoresFile) error
}

type Cache struct {
	probe     func(string) Result
	providers Providers
	home      string
	ttl       time.Duration
	mu        sync.Mutex
	entries   map[string]cacheEntry
	inflight  map[string]chan struct{}
}

func NewCache(home string, ttl time.Duration, providers Providers) *Cache {
	return &Cache{providers: providers, home: home, ttl: ttl, entries: map[string]cacheEntry{}, inflight: map[string]chan struct{}{}}
}

// Providers exposes the bound assembly checks for nested probes (the
// deployment closure re-enters Deployment with the same ports).
func (c *Cache) Providers() Providers {
	return c.providers
}

// BindProbe replaces the default probe with the assembly-supplied closure
// (deployment mode checks the opened runtime instead of the on-disk home).
func (c *Cache) BindProbe(probe func(surface string) Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.probe = probe
}

func (c *Cache) Surface(surface string) Result {
	if !knownReadinessSurface(surface) {
		return Result{Status: "not_ready", Surface: surface, ReasonCode: "SURFACE_UNKNOWN"}
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

		var result Result
		if c.probe != nil {
			result = c.probe(surface)
		} else {
			result = Probe(c.home, surface, c.providers)
		}
		c.mu.Lock()
		c.entries[surface] = cacheEntry{fingerprint: fingerprint, expires: time.Now().Add(c.ttl), result: result}
		delete(c.inflight, surface)
		close(done)
		c.mu.Unlock()
		return result
	}
}

func knownReadinessSurface(surface string) bool {
	return surface == "consumer" || surface == "writer" || surface == "search"
}

func (c *Cache) Overall() Result {
	for _, surface := range []string{"consumer", "writer", "search"} {
		if result := c.Surface(surface); result.Status != "ready" {
			result.Surface = "all"
			return result
		}
	}
	return Result{Status: "ready", Surface: "all"}
}

// readinessFingerprint invalidates the short probe cache immediately when
// the local home/config/evidence target changes. Remote-only changes are
// observed at the TTL boundary, keeping expensive backend checks off every
// load-balancer poll.
func readinessFingerprint(home, surface string) string {
	paths := []string{home, apphome.LayoutPath(home), apphome.StoresPath(home)}
	if surface == "writer" {
		paths = append(paths,
			filepath.Join(home, "access.jsonl"),
			filepath.Join(home, "feedback.jsonl"),
			filepath.Join(home, "retrieval.jsonl"),
			filepath.Join(home, "refine.jsonl"),
			filepath.Join(home, "access"),
			filepath.Join(home, "feedback"),
			filepath.Join(home, "retrieval"),
			filepath.Join(home, "refine"),
			filepath.Join(home, "writer.db"),
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

func Probe(home, surface string, providers Providers) Result {
	if !knownReadinessSurface(surface) {
		return Result{Status: "not_ready", Surface: surface, ReasonCode: "SURFACE_UNKNOWN"}
	}
	if !apphome.Ready(home) {
		return Result{Status: "not_ready", Surface: surface, ReasonCode: "HOME_NOT_INITIALIZED"}
	}
	stores, err := apphome.ReadStores(home)
	if err != nil || stores.ValidateProfile() != nil {
		return Result{Status: "not_ready", Surface: surface, ReasonCode: "HOME_CONFIGURATION_INVALID"}
	}
	// Validate local Registry state without opening every attached Repository:
	// one route's remote outage must not make unrelated routes globally unready.
	file, err := apphome.ReadHome(home)
	if err != nil {
		return Result{Status: "not_ready", Surface: surface, ReasonCode: "HOME_DISCOVERY_FAILED"}
	}
	sources := snapshot.NewRegistry()
	defer sources.Close()
	_, registry, err := apphome.OpenDatasets(home, file, sources)
	if err != nil {
		return Result{Status: "not_ready", Surface: surface, ReasonCode: "REGISTRY_STATE_UNAVAILABLE"}
	}
	defer registry.Close()
	if surface == "search" {
		if err := providers.SearchBackend(stores); err != nil {
			return Result{Status: "not_ready", Surface: surface, ReasonCode: "SEARCH_BACKEND_UNAVAILABLE"}
		}
	}
	if surface == "writer" {
		if _, err := snapshot.NewCommandLedger(snapshot.NewBoltCommandStore(filepath.Join(home, "writer.db"))); err != nil {
			return Result{Status: "not_ready", Surface: surface, ReasonCode: "COMMAND_LOG_UNAVAILABLE"}
		}
		if _, err := controlplane.NewFileControlState(filepath.Join(home, "control.json"), file.Registry.ID).Load(); err != nil {
			return Result{Status: "not_ready", Surface: surface, ReasonCode: "CONTROL_STATE_UNAVAILABLE"}
		}
		if err := EvidenceWriteProbe(home); err != nil {
			return Result{Status: "not_ready", Surface: surface, ReasonCode: "EVIDENCE_STORE_UNWRITABLE"}
		}
	}
	return Result{Status: "ready", Surface: surface}
}

// evidenceWriteProbe verifies every existing raw evidence target. For new
// targets it exercises the same directory durability primitive without
// appending a fake audit/access event or changing registry state. HTTP probes
// cache this result briefly, so fsync is not repeated for every poll.
func EvidenceWriteProbe(home string) error {
	for _, kind := range []string{"access", "feedback", "retrieval", "refine"} {
		legacy := filepath.Join(home, kind+".jsonl")
		if info, err := os.Stat(legacy); err == nil {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("%s evidence target is not a regular file", kind+".jsonl")
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		dir := filepath.Join(home, kind)
		if info, err := os.Stat(dir); err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s evidence partition is not a directory", kind)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		file, err := os.CreateTemp(dir, ".kc-ready-*")
		if err != nil {
			return err
		}
		name := file.Name()
		if _, err := file.Write([]byte("ready\n")); err != nil {
			_ = file.Close()
			_ = os.Remove(name)
			return err
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			_ = os.Remove(name)
			return err
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(name)
			return err
		}
		if err := os.Remove(name); err != nil {
			return err
		}
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

func Deployment(config apphome.DeploymentConfig, surface string, providers Providers) Result {
	fail := func(code string) Result {
		return Result{Status: "not_ready", Surface: surface, ReasonCode: code}
	}
	if err := apphome.ValidateDeploymentState(config); err != nil {
		return fail("DEPLOYMENT_STATE_UNAVAILABLE")
	}
	if surface == "search" {
		if err := providers.SearchBackend(config.Stores); err != nil {
			return fail("SEARCH_BACKEND_UNAVAILABLE")
		}
	}
	if surface == "writer" {
		if _, err := snapshot.OpenBoltCommandStore(filepath.Join(config.StateDir, "writer.db")); err != nil {
			return fail("COMMAND_LOG_UNAVAILABLE")
		}
		if _, err := controlplane.NewFileControlState(filepath.Join(config.StateDir, "control.json"), config.Registry.ID).Load(); err != nil {
			return fail("CONTROL_STATE_UNAVAILABLE")
		}
		if err := EvidenceWriteProbe(config.StateDir); err != nil {
			return fail("EVIDENCE_STORE_UNWRITABLE")
		}
	}
	return Result{Status: "ready", Surface: surface}
}
