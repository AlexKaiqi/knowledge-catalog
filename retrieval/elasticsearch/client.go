package elasticsearch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"kc/kernel"
)

type esEngine struct {
	base       string
	index      string
	http       *http.Client
	user       string
	pass       string
	apiKey     string
	repository kernel.RepositoryID
}

func (e *esEngine) Close() error { return nil }

func (e *esEngine) ProviderID() string       { return "elasticsearch" }
func (e *esEngine) ProviderRevision() string { return "elasticsearch-v3-search-mvp" }
func (e *esEngine) PhysicalDigest() kernel.Digest {
	return kernel.CanonicalDigest(map[string]any{"provider": e.ProviderID(), "revision": e.ProviderRevision(), "base": e.base})
}

func (e *esEngine) ensureIndex() error {
	mapping := map[string]any{
		"mappings": map[string]any{
			"properties": map[string]any{
				"object_id":  map[string]any{"type": "keyword"},
				"value_text": map[string]any{"type": "text"},
				"basis":      map[string]any{"type": "keyword"},
				"digest":     map[string]any{"type": "keyword"},
				"mode":       map[string]any{"type": "keyword"},
				"cause":      map[string]any{"type": "keyword"},
				"fields": map[string]any{
					"type": "nested",
					"properties": map[string]any{
						"path":       map[string]any{"type": "keyword"},
						"value":      map[string]any{"type": "keyword"},
						"text_value": map[string]any{"type": "text"},
					},
				},
			},
		},
	}
	status, body, err := e.do(http.MethodPut, "/"+e.index, mapping)
	if err != nil {
		return err
	}
	if status >= 400 && !strings.Contains(string(body), "resource_already_exists") {
		return fmt.Errorf("elasticsearch create index: %s", body)
	}
	return nil
}

func (e *esEngine) do(method, path string, payload any) (int, []byte, error) {
	var rdr io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, e.base+path, rdr)
	if err != nil {
		return 0, nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
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
	body, err := io.ReadAll(res.Body)
	return res.StatusCode, body, err
}
