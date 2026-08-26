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

	"kc/kernel"
)

const controlIndexName = "kc-projection-control-v1"

// openSearchEngine owns one Repository's managed OpenSearch projection.
type openSearchEngine struct {
	base       string
	prefix     string
	controlID  string
	http       *http.Client
	user       string
	pass       string
	apiKey     string
	repository kernel.RepositoryID
	mu         sync.RWMutex
}

func (e *openSearchEngine) Close() error { return nil }

func (e *openSearchEngine) ProviderID() string       { return "opensearch" }
func (e *openSearchEngine) ProviderRevision() string { return "opensearch-v1-typed-generation-pit" }
func (e *openSearchEngine) PhysicalDigest() kernel.Digest {
	return kernel.CanonicalDigest(map[string]any{
		"provider": e.ProviderID(), "revision": e.ProviderRevision(),
		"mapping": "typed-cells-v1", "compiler": "knowledge-units-v1",
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
			"dynamic": "strict",
			"properties": map[string]any{
				"repository":        map[string]any{"type": "keyword"},
				"active_index":      map[string]any{"type": "keyword"},
				"generation":        map[string]any{"type": "keyword"},
				"state":             map[string]any{"type": "keyword"},
				"basis":             map[string]any{"type": "keyword"},
				"access_digest":     map[string]any{"type": "keyword"},
				"physical_digest":   map[string]any{"type": "keyword"},
				"provider_revision": map[string]any{"type": "keyword"},
				"mode":              map[string]any{"type": "keyword"},
				"cause":             map[string]any{"type": "keyword"},
				"coverage":          map[string]any{"type": "double"},
				"object_count":      map[string]any{"type": "long"},
				"last_error":        map[string]any{"type": "keyword", "ignore_above": 4096},
			},
		},
	}
	status, body, err := e.do(http.MethodPut, "/"+controlIndexName, mapping)
	if err != nil {
		return err
	}
	if status >= 400 && !alreadyExists(body) {
		return fmt.Errorf("opensearch create control index: %s", body)
	}
	return nil
}

func projectionMapping() map[string]any {
	return map[string]any{
		"settings": map[string]any{"index": map[string]any{"number_of_shards": 1}},
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
						"object_ref": map[string]any{"type": "keyword"},
					},
				},
			},
		},
	}
}

func (e *openSearchEngine) createGeneration(name string) error {
	status, body, err := e.do(http.MethodPut, "/"+name, projectionMapping())
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
		return 0, nil, err
	}
	defer res.Body.Close()
	response, err := io.ReadAll(res.Body)
	return res.StatusCode, response, err
}
