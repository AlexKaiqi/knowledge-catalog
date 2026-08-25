package elasticsearch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
)

type esMetaDoc struct {
	Basis            string `json:"basis"`
	AccessDigest     string `json:"access_digest"`
	PhysicalDigest   string `json:"physical_digest"`
	ProviderRevision string `json:"provider_revision"`
	Mode             string `json:"mode"`
	Cause            string `json:"cause"`
}

type esDoc struct {
	ObjectID string    `json:"object_id"`
	Text     string    `json:"value_text"`
	Fields   []esField `json:"fields"`
}

type esField struct {
	Path      string `json:"path"`
	Value     string `json:"value"`
	TextValue string `json:"text_value"`
}

func (e *esEngine) LoadMeta() (index.Meta, error) {
	status, body, err := e.do(http.MethodGet, "/"+e.index+"/_doc/kc_meta", nil)
	if err != nil {
		return index.Meta{}, err
	}
	if status == http.StatusNotFound {
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
		Basis: kernel.CommitID(wrapped.Source.Basis), AccessDigest: kernel.Digest(wrapped.Source.AccessDigest),
		PhysicalDigest: kernel.Digest(wrapped.Source.PhysicalDigest), ProviderRevision: wrapped.Source.ProviderRevision,
		Mode: wrapped.Source.Mode, Cause: wrapped.Source.Cause,
	}, nil
}

func (e *esEngine) putMeta(meta index.Meta) error {
	status, body, err := e.do(http.MethodPut, "/"+e.index+"/_doc/kc_meta?refresh=true", esMetaDoc{
		Basis: string(meta.Basis), AccessDigest: string(meta.AccessDigest), PhysicalDigest: string(meta.PhysicalDigest),
		ProviderRevision: meta.ProviderRevision, Mode: meta.Mode, Cause: meta.Cause,
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

func (e *esEngine) Apply(upserts []index.CompiledDoc, deletes []knowledge.ObjectID, meta index.Meta) error {
	for _, id := range deletes {
		status, body, err := e.do(http.MethodDelete, "/"+e.index+"/_doc/"+esDocID(string(id))+"?refresh=true", nil)
		if err != nil {
			return err
		}
		if status >= 400 && status != http.StatusNotFound {
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
		fields = append(fields, esField{Path: pair[0], Value: pair[1], TextValue: pair[1]})
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

func esDocID(objectID string) string {
	s := strings.NewReplacer("/", "_", ":", "_").Replace(objectID)
	if s == "" {
		return "obj"
	}
	return s
}
