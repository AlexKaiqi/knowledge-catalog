package opensearch

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"kc/kernel"
)

const controlIndexName = "kc-projection-control-v1"
const maxResponseBytes = 64 << 20

// openSearchEngine owns one Repository's managed OpenSearch projection.
type openSearchEngine struct {
	base            string
	prefix          string
	controlID       string
	http            *http.Client
	user            string
	pass            string
	apiKey          string
	repository      kernel.RepositoryID
	mu              sync.RWMutex
	buildMu         sync.Mutex
	retireMu        sync.Mutex
	retired         map[string]*time.Timer
	primaryShards   int
	replicas        int
	refreshInterval string
}

func (e *openSearchEngine) Close() error {
	e.retireMu.Lock()
	defer e.retireMu.Unlock()
	for name, timer := range e.retired {
		timer.Stop()
		delete(e.retired, name)
	}
	return nil
}

func (e *openSearchEngine) ProviderID() string { return "opensearch" }
func (e *openSearchEngine) ProviderRevision() string {
	return "opensearch-v3-online-generations"
}
func (e *openSearchEngine) PhysicalDigest() kernel.Digest {
	return kernel.CanonicalDigest(map[string]any{
		"provider": e.ProviderID(), "revision": e.ProviderRevision(),
		"mapping": "typed-cells-v2-qualified-relations", "compiler": "knowledge-units-v2-binding-observation",
		"primaryShards": e.primaryShards, "replicas": e.replicas, "refreshInterval": e.refreshInterval,
	})
}

func projectionNames(id kernel.RepositoryID) (prefix, controlID string) {
	digest := sha256.Sum256([]byte(id))
	encoded := hex.EncodeToString(digest[:])
	return "kc-proj-" + encoded[:24], encoded
}

func documentID(objectID string) string {
	digest := sha256.Sum256([]byte(objectID))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (e *openSearchEngine) ensureControlIndex() error {
	mapping := map[string]any{
		"settings": map[string]any{"index": map[string]any{"number_of_shards": 1}},
		"mappings": map[string]any{
			"dynamic":    "strict",
			"properties": controlProperties(),
		},
	}
	status, body, err := e.do(http.MethodPut, "/"+controlIndexName, mapping)
	if err != nil {
		return err
	}
	if status >= 400 && !alreadyExists(body) {
		return fmt.Errorf("opensearch create control index: %s", body)
	}
	if status >= 400 {
		status, body, err = e.do(http.MethodPut, "/"+controlIndexName+"/_mapping", map[string]any{"properties": controlProperties()})
		if err != nil {
			return err
		}
		if status >= 400 {
			return fmt.Errorf("opensearch update control mapping: %s", body)
		}
	}
	return nil
}

func controlProperties() map[string]any {
	return map[string]any{
		"repository":          map[string]any{"type": "keyword"},
		"active_index":        map[string]any{"type": "keyword"},
		"generation":          map[string]any{"type": "keyword"},
		"projection_revision": map[string]any{"type": "keyword"},
		"state":               map[string]any{"type": "keyword"},
		"basis":               map[string]any{"type": "keyword"},
		"access_digest":       map[string]any{"type": "keyword"},
		"observation_digest":  map[string]any{"type": "keyword"},
		"physical_digest":     map[string]any{"type": "keyword"},
		"provider_revision":   map[string]any{"type": "keyword"},
		"mode":                map[string]any{"type": "keyword"},
		"cause":               map[string]any{"type": "keyword"},
		"coverage":            map[string]any{"type": "double"},
		"object_count":        map[string]any{"type": "long"},
		"last_error":          map[string]any{"type": "keyword", "ignore_above": 4096},
	}
}

func (e *openSearchEngine) projectionMapping() map[string]any {
	return map[string]any{
		"settings": map[string]any{"index": map[string]any{
			"number_of_shards": e.primaryShards, "number_of_replicas": e.replicas,
			"refresh_interval": e.refreshInterval,
		}},
		"mappings": map[string]any{
			"dynamic": "strict",
			"properties": map[string]any{
				"object_id":       map[string]any{"type": "keyword"},
				"kind":            map[string]any{"type": "keyword"},
				"eligible_fields": map[string]any{"type": "keyword"},
				"all_text":        map[string]any{"type": "text"},
				"object_digest":   map[string]any{"type": "keyword"},
				"cells": map[string]any{
					"type": "nested",
					"properties": map[string]any{
						"field":         map[string]any{"type": "keyword"},
						"string_value":  map[string]any{"type": "keyword"},
						"text_value":    map[string]any{"type": "text"},
						"long_value":    map[string]any{"type": "long"},
						"double_value":  map[string]any{"type": "double"},
						"boolean_value": map[string]any{"type": "boolean"},
						"date_value":    map[string]any{"type": "date", "format": "strict_date_optional_time||strict_date"},
					},
				},
				"relation_type":      map[string]any{"type": "keyword"},
				"relation_direction": map[string]any{"type": "keyword"},
				"relation_endpoints": map[string]any{
					"type": "nested",
					"properties": map[string]any{
						"role":       map[string]any{"type": "keyword"},
						"repository": map[string]any{"type": "keyword"},
						"object_id":  map[string]any{"type": "keyword"},
					},
				},
			},
		},
	}
}

func (e *openSearchEngine) createGeneration(name string) error {
	status, body, err := e.do(http.MethodPut, "/"+name, e.projectionMapping())
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("opensearch create generation: %s", body)
	}
	return nil
}

func (e *openSearchEngine) refresh(name string) error {
	status, body, err := e.do(http.MethodPost, "/"+name+"/_refresh", nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("opensearch refresh generation: %s", body)
	}
	return nil
}

func alreadyExists(body []byte) bool {
	return strings.Contains(string(body), "resource_already_exists_exception")
}

func (e *openSearchEngine) do(method, path string, payload any) (int, []byte, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
	}
	contentType := ""
	if payload != nil {
		contentType = "application/json"
	}
	return e.doBytes(method, path, body, contentType)
}

func (e *openSearchEngine) doBytes(method, path string, body []byte, contentType string) (int, []byte, error) {
	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, e.base+path, rdr)
	if err != nil {
		return 0, nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if e.apiKey != "" {
		req.Header.Set("Authorization", "ApiKey "+e.apiKey)
	} else if e.pass != "" {
		req.SetBasicAuth(e.user, e.pass)
	}
	res, err := e.http.Do(req)
	if err != nil {
		return 0, nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "opensearch %s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	response, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return res.StatusCode, nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "opensearch read %s response: %v", path, err)
	}
	if len(response) > maxResponseBytes {
		return res.StatusCode, nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "opensearch %s response exceeds %d bytes", path, maxResponseBytes)
	}
	if res.StatusCode == http.StatusRequestTimeout || res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= http.StatusInternalServerError {
		return res.StatusCode, response, kernel.Fail(kernel.ErrTemporaryUnavailable, "opensearch %s %s returned HTTP %d", method, path, res.StatusCode)
	}
	return res.StatusCode, response, nil
}
