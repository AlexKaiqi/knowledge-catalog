package scale

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"kc/index"
	"kc/kernel"
	"kc/reader"
	"kc/repository"
)

const (
	EnvElasticsearchPassword = "KC_ELASTICSEARCH_PASSWORD"
	EnvElasticsearchAPIKey   = "KC_ELASTICSEARCH_API_KEY"
	defaultElasticsearchURL  = "http://127.0.0.1:9200"
)

// ElasticsearchConfig is non-secret cluster location for full-text (MATCH).
// Not StarRocks, not authority. Password and API key stay in the environment.
type ElasticsearchConfig struct {
	URL      string `json:"url,omitempty" yaml:"url,omitempty"`
	User     string `json:"user,omitempty" yaml:"user,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
	APIKey   string `json:"apiKey,omitempty" yaml:"apiKey,omitempty"`
}

// WithDefaults fills the local sandbox URL. It does not set credentials.
func (c ElasticsearchConfig) WithDefaults() ElasticsearchConfig {
	if strings.TrimSpace(c.URL) == "" {
		c.URL = defaultElasticsearchURL
	}
	return c
}

// RejectSecrets refuses passwords or API keys in the config file / URL.
func (c ElasticsearchConfig) RejectSecrets() error {
	if err := repository.RejectConfiguredSecret("elasticsearch", c.URL, EnvElasticsearchPassword); err != nil {
		return err
	}
	if strings.TrimSpace(c.Password) != "" {
		return fmt.Errorf("elasticsearch connection config must not contain secrets; set %s", EnvElasticsearchPassword)
	}
	if strings.TrimSpace(c.APIKey) != "" {
		return fmt.Errorf("elasticsearch connection config must not contain secrets; set %s", EnvElasticsearchAPIKey)
	}
	return nil
}

// OpenElasticsearch returns an index.EngineOpener for full-text SEARCH (MATCH).
// Column filter/aggregation at scale is StarRocks, not this engine.
//
// Args:
//
//	cfg: URL and optional basic-auth user. Password is KC_ELASTICSEARCH_PASSWORD; API key is KC_ELASTICSEARCH_API_KEY.
//
// Returns:
//
//	an opener that builds one index per repository.
func OpenElasticsearch(cfg ElasticsearchConfig) index.EngineOpener {
	return func(_ string, id kernel.RepositoryID) (index.Engine, error) {
		if err := cfg.RejectSecrets(); err != nil {
			return nil, err
		}
		cfg = cfg.WithDefaults()
		baseURL := strings.TrimRight(cfg.URL, "/")
		eng := &esEngine{
			base:   baseURL,
			index:  "kc-proj-" + strings.ToLower(index.SanitizeID(string(id))),
			http:   &http.Client{Timeout: 12 * time.Second},
			user:   cfg.User,
			pass:   strings.TrimSpace(os.Getenv(EnvElasticsearchPassword)),
			apiKey: strings.TrimSpace(os.Getenv(EnvElasticsearchAPIKey)),
		}
		if err := eng.ensureIndex(); err != nil {
			return nil, err
		}
		return eng, nil
	}
}

type esEngine struct {
	base   string
	index  string
	http   *http.Client
	user   string
	pass   string
	apiKey string
}

type esMetaDoc struct {
	Basis  string `json:"basis"`
	Digest string `json:"digest"`
	Mode   string `json:"mode"`
	Cause  string `json:"cause"`
}

type esDoc struct {
	ObjectID string    `json:"object_id"`
	Text     string    `json:"value_text"`
	Fields   []esField `json:"fields"`
}

type esField struct {
	Path  string `json:"path"`
	Value string `json:"value"`
}

func (e *esEngine) Close() error { return nil }

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
						"path":  map[string]any{"type": "keyword"},
						"value": map[string]any{"type": "keyword"},
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
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		rdr = bytes.NewReader(b)
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

func (e *esEngine) LoadMeta() (index.Meta, error) {
	status, body, err := e.do(http.MethodGet, "/"+e.index+"/_doc/kc_meta", nil)
	if err != nil {
		return index.Meta{}, err
	}
	if status == 404 {
		return index.Meta{}, nil
	}
	if status >= 400 {
		return index.Meta{}, fmt.Errorf("elasticsearch meta: %s", body)
	}
	var wrapped struct {
		Source esMetaDoc `json:"_source"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return index.Meta{}, err
	}
	return index.Meta{
		Basis:  kernel.CommitID(wrapped.Source.Basis),
		Digest: kernel.Digest(wrapped.Source.Digest),
		Mode:   wrapped.Source.Mode,
		Cause:  wrapped.Source.Cause,
	}, nil
}

func (e *esEngine) putMeta(meta index.Meta) error {
	status, body, err := e.do(http.MethodPut, "/"+e.index+"/_doc/kc_meta?refresh=true", esMetaDoc{
		Basis:  string(meta.Basis),
		Digest: string(meta.Digest),
		Mode:   meta.Mode,
		Cause:  meta.Cause,
	})
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("elasticsearch put meta: %s", body)
	}
	return nil
}

func (e *esEngine) Rebuild(docs []index.CompiledDoc, meta index.Meta) error {
	status, body, err := e.do(http.MethodPost, "/"+e.index+"/_delete_by_query?refresh=true", map[string]any{
		"query": map[string]any{"match_all": map[string]any{}},
	})
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("elasticsearch wipe: %s", body)
	}
	for _, doc := range docs {
		if err := e.putDoc(doc); err != nil {
			return err
		}
	}
	return e.putMeta(meta)
}

func (e *esEngine) Apply(upserts []index.CompiledDoc, deletes []kernel.ObjectID, meta index.Meta) error {
	for _, id := range deletes {
		status, body, err := e.do(http.MethodDelete, "/"+e.index+"/_doc/"+esDocID(string(id))+"?refresh=true", nil)
		if err != nil {
			return err
		}
		if status >= 400 && status != 404 {
			return fmt.Errorf("elasticsearch delete: %s", body)
		}
	}
	for _, doc := range upserts {
		if err := e.putDoc(doc); err != nil {
			return err
		}
	}
	return e.putMeta(meta)
}

func (e *esEngine) putDoc(doc index.CompiledDoc) error {
	fields := make([]esField, 0, len(doc.Fields))
	for _, pair := range doc.Fields {
		fields = append(fields, esField{Path: pair[0], Value: pair[1]})
	}
	status, body, err := e.do(http.MethodPut, "/"+e.index+"/_doc/"+esDocID(string(doc.ObjectID))+"?refresh=true", esDoc{
		ObjectID: string(doc.ObjectID), Text: doc.Text, Fields: fields,
	})
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("elasticsearch put doc: %s", body)
	}
	return nil
}

func (e *esEngine) Count() (int, error) {
	status, body, err := e.do(http.MethodPost, "/"+e.index+"/_count", map[string]any{
		"query": map[string]any{"exists": map[string]any{"field": "object_id"}},
	})
	if err != nil {
		return 0, err
	}
	if status >= 400 {
		return 0, fmt.Errorf("elasticsearch count: %s", body)
	}
	var out struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

func (e *esEngine) Search(req reader.SearchRequest, spec reader.IndexSpec) ([]kernel.ObjectID, error) {
	var filters []map[string]any
	for _, c := range req.Clauses {
		if c.Op == reader.OpSort {
			continue
		}
		q, err := esClause(c)
		if err != nil {
			return nil, err
		}
		filters = append(filters, q)
	}
	if len(filters) == 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "search requires a locating clause")
	}
	payload := map[string]any{
		"size":    500,
		"_source": []string{"object_id"},
		"query":   map[string]any{"bool": map[string]any{"filter": filters}},
	}
	status, body, err := e.do(http.MethodPost, "/"+e.index+"/_search", payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("elasticsearch search: %s", body)
	}
	var out struct {
		Hits struct {
			Hits []struct {
				Source struct {
					ObjectID string `json:"object_id"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	var ids []kernel.ObjectID
	for _, hit := range out.Hits.Hits {
		if hit.Source.ObjectID == "" {
			continue
		}
		ids = append(ids, kernel.ObjectID(hit.Source.ObjectID))
	}
	return ids, nil
}

func esClause(c reader.SearchClause) (map[string]any, error) {
	nested := func(path, value string) map[string]any {
		return map[string]any{"nested": map[string]any{
			"path": "fields",
			"query": map[string]any{"bool": map[string]any{"must": []map[string]any{
				{"term": map[string]any{"fields.path": path}},
				{"term": map[string]any{"fields.value": value}},
			}}},
		}}
	}
	switch c.Op {
	case reader.OpMatch:
		if c.Path == "" {
			return map[string]any{"match": map[string]any{"value_text": c.Value}}, nil
		}
		return map[string]any{"nested": map[string]any{
			"path": "fields",
			"query": map[string]any{"bool": map[string]any{"must": []map[string]any{
				{"term": map[string]any{"fields.path": c.Path}},
				{"wildcard": map[string]any{"fields.value": "*" + strings.ToLower(c.Value) + "*"}},
			}}},
		}}, nil
	case reader.OpEQ:
		return nested(c.Path, c.Value), nil
	case reader.OpIN:
		should := make([]map[string]any, 0, len(c.Values))
		for _, v := range c.Values {
			should = append(should, nested(c.Path, v))
		}
		return map[string]any{"bool": map[string]any{"should": should, "minimum_should_match": 1}}, nil
	case reader.OpExists:
		return map[string]any{"nested": map[string]any{
			"path":  "fields",
			"query": map[string]any{"term": map[string]any{"fields.path": c.Path}},
		}}, nil
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "elasticsearch projection does not implement %s", c.Op)
	}
}

func esDocID(objectID string) string {
	s := strings.ReplaceAll(objectID, "/", "_")
	s = strings.ReplaceAll(s, ":", "_")
	if s == "" {
		return "obj"
	}
	return s
}
