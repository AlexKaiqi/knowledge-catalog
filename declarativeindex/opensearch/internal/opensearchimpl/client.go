package opensearchimpl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	retrieval "kc/discovery"
	"kc/kernel"
)

const controlIndexName = "kc-projection-control-v1"
const maxResponseBytes = 64 << 20

// openSearchEngine owns one Repository's managed OpenSearch projection.
type openSearchEngine struct {
	base            string
	prefix          string
	controlID       string
	controlIndex    string
	namespace       string
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
	// embedder serves request-time query vectors and projection-time document
	// vectors. Nil keeps the projection purely lexical: no knn mapping, no
	// vector window, and semantic recall fails closed.
	embedder        retrieval.Embedder
	vectorDimension int
	vectorModel     string
	// denseVector is set when the cluster rejects OpenSearch knn settings and
	// serves Elasticsearch dense_vector plus /_pit instead.
	denseVector atomic.Bool
	vectorKnown atomic.Bool
}

// VectorReady reports whether this engine derives a k-NN vector surface.
func (e *openSearchEngine) VectorReady() bool {
	return e != nil && e.embedder != nil && e.vectorDimension > 0
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
	return "opensearch-v6-declaration-member-fields"
}
func (e *openSearchEngine) PhysicalDigest() kernel.Digest {
	identity := map[string]any{
		"provider": e.ProviderID(), "revision": e.ProviderRevision(),
		"mapping": "typed-cells-v4-access-faces", "compiler": "object-units-v4-member-construction",
		"primaryShards": e.primaryShards, "replicas": e.replicas, "refreshInterval": e.refreshInterval,
	}
	if e.namespace != "" {
		identity["namespace"] = e.namespace
	}
	if e.VectorReady() {
		identity["embeddingModel"], identity["embeddingDimensions"] = e.vectorModel, e.vectorDimension
		if e.denseVector.Load() {
			identity["vectorStore"] = "dense_vector"
		}
	}
	return kernel.CanonicalDigest(identity)
}

func namespaceDigest(namespace string) kernel.Digest {
	if namespace == "" {
		return ""
	}
	return kernel.CanonicalDigest(namespace)
}

func (e *openSearchEngine) controlIndexPath() string {
	if e.controlIndex == "" {
		return controlIndexName
	}
	return e.controlIndex
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
	status, body, err := e.do(http.MethodPut, "/"+e.controlIndexPath(), mapping)
	if err != nil {
		return err
	}
	if status >= 400 && !alreadyExists(body) {
		return fmt.Errorf("opensearch create control index: %s", body)
	}
	if status >= 400 {
		status, body, err = e.do(http.MethodPut, "/"+e.controlIndexPath()+"/_mapping", map[string]any{"properties": controlProperties()})
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
		"schema_adoptions":    map[string]any{"type": "object", "enabled": false},
	}
}

func (e *openSearchEngine) projectionMapping() map[string]any {
	settings := map[string]any{
		"number_of_shards": e.primaryShards, "number_of_replicas": e.replicas,
		"refresh_interval": e.refreshInterval,
	}
	properties := map[string]any{
		"source_repository": map[string]any{"type": "keyword"},
		"source_commit":     map[string]any{"type": "keyword"},
		"object_id":         map[string]any{"type": "keyword"},
		"kind":              map[string]any{"type": "keyword"},
		"eligible_fields":   map[string]any{"type": "keyword"},
		"all_text":          map[string]any{"type": "text"},
		"object_digest":     map[string]any{"type": "keyword"},
		"cells": map[string]any{
			"type": "nested",
			"properties": map[string]any{
				"field":         map[string]any{"type": "keyword"},
				"access":        map[string]any{"type": "keyword"},
				"string_value":  map[string]any{"type": "keyword"},
				"text_value":    map[string]any{"type": "text"},
				"long_value":    map[string]any{"type": "long"},
				"double_value":  map[string]any{"type": "double"},
				"boolean_value": map[string]any{"type": "boolean"},
				"date_value":    map[string]any{"type": "keyword"},
			},
		},
		"relation_type":      map[string]any{"type": "keyword"},
		"relation_direction": map[string]any{"type": "keyword"},
		"relation_endpoints": map[string]any{
			"type": "nested",
			"properties": map[string]any{
				"role":      map[string]any{"type": "keyword"},
				"object_id": map[string]any{"type": "keyword"},
			},
		},
	}
	if e.VectorReady() {
		vector := map[string]any{"type": "knn_vector", "dimension": e.vectorDimension, "method": map[string]any{"name": "hnsw", "space_type": "cosinesimil", "engine": "lucene"}}
		if e.denseVector.Load() {
			vector = map[string]any{"type": "dense_vector", "dims": e.vectorDimension, "index": true, "similarity": "cosine"}
		} else {
			settings["knn"] = true
		}
		properties["semantic_vectors"] = map[string]any{"type": "nested", "properties": map[string]any{
			"member": map[string]any{"type": "keyword"}, "model": map[string]any{"type": "keyword"},
			"vector": vector,
		}}
	}
	return map[string]any{
		"settings": map[string]any{"index": settings},
		"mappings": map[string]any{
			"dynamic":    "strict",
			"properties": properties,
		},
	}
}

func (e *openSearchEngine) createGeneration(name string) error {
	e.detectVectorStore(context.Background())
	status, body, err := e.do(http.MethodPut, "/"+name, e.projectionMapping())
	if err != nil {
		return err
	}
	if status >= 400 && e.VectorReady() && bytes.Contains(body, []byte("unknown setting [index.knn]")) {
		e.denseVector.Store(true)
		e.vectorKnown.Store(true)
		status, body, err = e.do(http.MethodPut, "/"+name, e.projectionMapping())
		if err != nil {
			return err
		}
	}
	if status >= 400 {
		return fmt.Errorf("opensearch create generation: %s", body)
	}
	return nil
}

// detectVectorStore records whether this cluster is Elasticsearch. OpenSearch
// keeps knn_vector. Elasticsearch rejects index.knn and serves dense_vector.
func (e *openSearchEngine) detectVectorStore(ctx context.Context) {
	if e.vectorKnown.Load() || !e.VectorReady() {
		return
	}
	status, body, err := e.doContext(ctx, http.MethodGet, "/", nil)
	if err != nil || status >= 400 {
		return
	}
	var info struct {
		Tagline string `json:"tagline"`
		Version struct {
			Distribution string `json:"distribution"`
		} `json:"version"`
	}
	if json.Unmarshal(body, &info) != nil {
		return
	}
	e.vectorKnown.Store(true)
	if info.Version.Distribution != "opensearch" && strings.Contains(info.Tagline, "You Know, for Search") {
		e.denseVector.Store(true)
	}
}

func pitRouteMissing(body []byte) bool {
	return bytes.Contains(body, []byte("no handler found"))
}

func (e *openSearchEngine) refresh(name string) error {
	status, body, err := e.do(http.MethodPost, "/"+name+"/_refresh", nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("opensearch refresh generation: %s", body)
	}
	var response struct {
		Shards *shardResponse `json:"_shards"`
	}
	if err := decodeJSON(body, &response); err != nil {
		return err
	}
	return response.Shards.check("refresh")
}

// Incremental publication holds the write lock while awaiting shard refresh.
// A request deadline must also bound contention before the HTTP operation.
func (e *openSearchEngine) readLockContext(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "opensearch retrieval cancelled: %v", err)
		}
		if e.mu.TryRLock() {
			return nil
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "opensearch retrieval cancelled: %v", ctx.Err())
		case <-timer.C:
		}
	}
}

func alreadyExists(body []byte) bool {
	return strings.Contains(string(body), "resource_already_exists_exception")
}

func (e *openSearchEngine) do(method, path string, payload any) (int, []byte, error) {
	return e.doContext(context.Background(), method, path, payload)
}

func (e *openSearchEngine) doContext(ctx context.Context, method, path string, payload any) (int, []byte, error) {
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
	return e.doBytesContext(ctx, method, path, body, contentType)
}

func (e *openSearchEngine) doBytes(method, path string, body []byte, contentType string) (int, []byte, error) {
	return e.doBytesContext(context.Background(), method, path, body, contentType)
}

func (e *openSearchEngine) doBytesContext(ctx context.Context, method, path string, body []byte, contentType string) (int, []byte, error) {
	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, e.base+path, rdr)
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
