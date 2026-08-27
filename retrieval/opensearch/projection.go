package opensearch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
)

type controlDoc struct {
	Repository         string  `json:"repository"`
	ActiveIndex        string  `json:"active_index"`
	Generation         string  `json:"generation"`
	ProjectionRevision string  `json:"projection_revision,omitempty"`
	State              string  `json:"state"`
	Basis              string  `json:"basis"`
	AccessDigest       string  `json:"access_digest"`
	ObservationDigest  string  `json:"observation_digest,omitempty"`
	PhysicalDigest     string  `json:"physical_digest"`
	ProviderRevision   string  `json:"provider_revision"`
	Mode               string  `json:"mode"`
	Cause              string  `json:"cause"`
	Coverage           float64 `json:"coverage"`
	ObjectCount        int     `json:"object_count"`
	LastError          string  `json:"last_error"`
}

type controlVersion struct {
	SeqNo       int64
	PrimaryTerm int64
}

type osDoc struct {
	ObjectID          string               `json:"object_id"`
	Kind              string               `json:"kind"`
	EligibleFields    []string             `json:"eligible_fields"`
	AllText           string               `json:"all_text"`
	Cells             []osCell             `json:"cells"`
	RelationType      string               `json:"relation_type"`
	RelationDirection string               `json:"relation_direction"`
	RelationEndpoints []osRelationEndpoint `json:"relation_endpoints"`
	ObjectDigest      string               `json:"object_digest"`
}

type osCell struct {
	Field        string   `json:"field"`
	StringValue  *string  `json:"string_value,omitempty"`
	TextValue    string   `json:"text_value,omitempty"`
	LongValue    *int64   `json:"long_value,omitempty"`
	DoubleValue  *float64 `json:"double_value,omitempty"`
	BooleanValue *bool    `json:"boolean_value,omitempty"`
	DateValue    string   `json:"date_value,omitempty"`
}

type osRelationEndpoint struct {
	Role      string `json:"role"`
	ObjectRef string `json:"object_ref"`
}

func encodeDoc(doc index.CompiledDoc) osDoc {
	out := osDoc{
		ObjectID: string(doc.ObjectID), Kind: string(doc.Kind), EligibleFields: doc.EligibleFields,
		AllText: doc.Text, ObjectDigest: string(doc.ObjectDigest), Cells: make([]osCell, 0, len(doc.Cells)),
		RelationEndpoints: []osRelationEndpoint{},
	}
	for _, cell := range doc.Cells {
		out.Cells = append(out.Cells, osCell{
			Field: cell.Field, StringValue: cell.StringValue, TextValue: cell.TextValue,
			LongValue: cell.LongValue, DoubleValue: cell.DoubleValue,
			BooleanValue: cell.BooleanValue, DateValue: cell.DateValue,
		})
	}
	if doc.Relation != nil {
		out.RelationType = doc.Relation.Type
		out.RelationDirection = string(doc.Relation.Direction)
		for _, endpoint := range doc.Relation.Endpoints {
			out.RelationEndpoints = append(out.RelationEndpoints, osRelationEndpoint{Role: endpoint.Role, ObjectRef: string(endpoint.ObjectRef)})
		}
	}
	return out
}

func metaFromControl(control controlDoc) index.Meta {
	return index.Meta{
		Basis: kernel.CommitID(control.Basis), AccessDigest: kernel.Digest(control.AccessDigest),
		ObservationDigest: kernel.Digest(control.ObservationDigest),
		PhysicalDigest:    kernel.Digest(control.PhysicalDigest), ProviderRevision: control.ProviderRevision,
		Generation: control.Generation, State: control.State, Coverage: control.Coverage,
		Revision: control.ProjectionRevision,
		Mode:     control.Mode, Cause: control.Cause,
	}
}

func controlFromMeta(repository kernel.RepositoryID, active, generation, state string, meta index.Meta, count int) controlDoc {
	coverage := meta.Coverage
	if coverage == 0 {
		coverage = 1
	}
	return controlDoc{
		Repository: string(repository), ActiveIndex: active, Generation: generation, State: state,
		ProjectionRevision: meta.Revision,
		Basis:              string(meta.Basis), AccessDigest: string(meta.AccessDigest), PhysicalDigest: string(meta.PhysicalDigest),
		ObservationDigest: string(meta.ObservationDigest),
		ProviderRevision:  meta.ProviderRevision, Mode: meta.Mode, Cause: meta.Cause,
		Coverage: coverage, ObjectCount: count,
	}
}

func (e *openSearchEngine) loadControl() (controlDoc, *controlVersion, error) {
	status, body, err := e.do(http.MethodGet, "/"+controlIndexName+"/_doc/"+url.PathEscape(e.controlID), nil)
	if err != nil {
		return controlDoc{}, nil, err
	}
	if status == http.StatusNotFound {
		return controlDoc{}, nil, nil
	}
	if status >= 400 {
		return controlDoc{}, nil, fmt.Errorf("opensearch load projection control: %s", body)
	}
	var wrapped struct {
		Source      controlDoc `json:"_source"`
		SeqNo       int64      `json:"_seq_no"`
		PrimaryTerm int64      `json:"_primary_term"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return controlDoc{}, nil, err
	}
	return wrapped.Source, &controlVersion{SeqNo: wrapped.SeqNo, PrimaryTerm: wrapped.PrimaryTerm}, nil
}

func (e *openSearchEngine) putControl(control controlDoc, expected *controlVersion) (*controlVersion, error) {
	path := "/" + controlIndexName + "/_doc/" + url.PathEscape(e.controlID) + "?refresh=true"
	if expected == nil {
		path += "&op_type=create"
	} else {
		path += "&if_seq_no=" + strconv.FormatInt(expected.SeqNo, 10) + "&if_primary_term=" + strconv.FormatInt(expected.PrimaryTerm, 10)
	}
	status, body, err := e.do(http.MethodPut, path, control)
	if err != nil {
		return nil, err
	}
	if status == http.StatusConflict {
		return nil, kernel.Fail(kernel.ErrNonFastForward, "projection control changed concurrently")
	}
	if status >= 400 {
		return nil, fmt.Errorf("opensearch publish projection control: %s", body)
	}
	var response struct {
		SeqNo       int64 `json:"_seq_no"`
		PrimaryTerm int64 `json:"_primary_term"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	return &controlVersion{SeqNo: response.SeqNo, PrimaryTerm: response.PrimaryTerm}, nil
}

func (e *openSearchEngine) LoadMeta() (index.Meta, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	control, _, err := e.loadControl()
	if err != nil || control.ActiveIndex == "" {
		return index.Meta{}, err
	}
	return metaFromControl(control), nil
}

func (e *openSearchEngine) Rebuild(docs []index.CompiledDoc, meta index.Meta) error {
	session, err := e.BeginRebuild(meta)
	if err != nil {
		return err
	}
	if err := session.Append(docs); err != nil {
		return session.Abort(err)
	}
	return session.Commit()
}

type rebuildSession struct {
	engine        *openSearchEngine
	old           controlDoc
	buildVersion  *controlVersion
	physicalIndex string
	generation    string
	meta          index.Meta
	expected      int
	done          bool
}

func (e *openSearchEngine) BeginRebuild(meta index.Meta) (index.RebuildSession, error) {
	e.mu.Lock()
	old, version, err := e.loadControl()
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	generation := strconv.FormatInt(time.Now().UnixNano(), 36)
	physicalIndex := e.prefix + "-g-" + generation
	if err := e.createGeneration(physicalIndex); err != nil {
		e.mu.Unlock()
		return nil, err
	}
	building := old
	if building.Repository == "" {
		building = controlFromMeta(e.repository, "", generation, index.ProjectionStateBuilding, meta, 0)
	} else {
		building.State = index.ProjectionStateBuilding
		building.Generation = generation
		building.LastError = ""
	}
	buildVersion, err := e.putControl(building, version)
	if err != nil {
		e.dropGeneration(physicalIndex)
		e.mu.Unlock()
		return nil, err
	}
	return &rebuildSession{
		engine: e, old: old, buildVersion: buildVersion, physicalIndex: physicalIndex,
		generation: generation, meta: meta,
	}, nil
}

func (s *rebuildSession) Append(docs []index.CompiledDoc) error {
	if s.done {
		return kernel.Fail(kernel.ErrPreconditionFailed, "OpenSearch rebuild session is closed")
	}
	if len(docs) == 0 {
		return nil
	}
	if err := s.engine.bulk(s.physicalIndex, docs, nil); err != nil {
		return err
	}
	s.expected += len(docs)
	return nil
}

func (s *rebuildSession) Commit() error {
	if s.done {
		return kernel.Fail(kernel.ErrPreconditionFailed, "OpenSearch rebuild session is closed")
	}
	if err := s.engine.refresh(s.physicalIndex); err != nil {
		return s.Abort(err)
	}
	count, err := s.engine.countIndex(s.physicalIndex)
	if err != nil {
		return s.Abort(err)
	}
	if count != s.expected {
		return s.Abort(fmt.Errorf("opensearch generation count %d does not match compiled count %d", count, s.expected))
	}
	ready := controlFromMeta(s.engine.repository, s.physicalIndex, s.generation, index.ProjectionStateReady, s.meta, count)
	if _, err := s.engine.putControl(ready, s.buildVersion); err != nil {
		return s.Abort(err)
	}
	s.done = true
	s.engine.mu.Unlock()
	return nil
}

func (s *rebuildSession) Abort(buildErr error) error {
	if s.done {
		return buildErr
	}
	fallback := s.old
	if fallback.ActiveIndex == "" {
		fallback = controlFromMeta(s.engine.repository, "", s.generation, index.ProjectionStateFailed, s.meta, 0)
	} else {
		fallback.State = index.ProjectionStateReady
	}
	fallback.LastError = buildErr.Error()
	_, _ = s.engine.putControl(fallback, s.buildVersion)
	s.engine.dropGeneration(s.physicalIndex)
	s.done = true
	s.engine.mu.Unlock()
	return buildErr
}

func (e *openSearchEngine) Apply(upserts []index.CompiledDoc, deletes []knowledge.ObjectID, meta index.Meta) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	control, version, err := e.loadControl()
	if err != nil {
		return err
	}
	if control.ActiveIndex == "" || control.State != index.ProjectionStateReady {
		return kernel.Fail(kernel.ErrPreconditionFailed, "opensearch projection is not READY")
	}
	updating := control
	updating.State = index.ProjectionStateUpdating
	updating.LastError = ""
	updateVersion, err := e.putControl(updating, version)
	if err != nil {
		return err
	}
	fail := func(updateErr error) error {
		failed := updating
		failed.State = index.ProjectionStateFailed
		failed.LastError = updateErr.Error()
		_, _ = e.putControl(failed, updateVersion)
		return updateErr
	}
	if err := e.bulk(control.ActiveIndex, upserts, deletes); err != nil {
		return fail(err)
	}
	if err := e.refresh(control.ActiveIndex); err != nil {
		return fail(err)
	}
	count, err := e.countIndex(control.ActiveIndex)
	if err != nil {
		return fail(err)
	}
	ready := controlFromMeta(e.repository, control.ActiveIndex, control.Generation, index.ProjectionStateReady, meta, count)
	if _, err := e.putControl(ready, updateVersion); err != nil {
		return fail(err)
	}
	return nil
}

func (e *openSearchEngine) bulk(physicalIndex string, docs []index.CompiledDoc, deletes []knowledge.ObjectID) error {
	const batchSize = 500
	for start := 0; start < len(deletes); start += batchSize {
		end := start + batchSize
		if end > len(deletes) {
			end = len(deletes)
		}
		var body bytes.Buffer
		for _, id := range deletes[start:end] {
			writeNDJSON(&body, map[string]any{"delete": map[string]any{"_index": physicalIndex, "_id": documentID(string(id))}})
		}
		if err := e.sendBulk(body.Bytes()); err != nil {
			return err
		}
	}
	for start := 0; start < len(docs); start += batchSize {
		end := start + batchSize
		if end > len(docs) {
			end = len(docs)
		}
		var body bytes.Buffer
		for _, doc := range docs[start:end] {
			writeNDJSON(&body, map[string]any{"index": map[string]any{"_index": physicalIndex, "_id": documentID(string(doc.ObjectID))}})
			writeNDJSON(&body, encodeDoc(doc))
		}
		if err := e.sendBulk(body.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func writeNDJSON(buffer *bytes.Buffer, value any) {
	encoded, _ := json.Marshal(value)
	buffer.Write(encoded)
	buffer.WriteByte('\n')
}

func (e *openSearchEngine) sendBulk(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	status, body, err := e.doBytes(http.MethodPost, "/_bulk", payload, "application/x-ndjson")
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("opensearch bulk: %s", body)
	}
	var response struct {
		Errors bool `json:"errors"`
		Items  []map[string]struct {
			Status int            `json:"status"`
			Error  map[string]any `json:"error"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}
	if !response.Errors {
		return nil
	}
	for _, item := range response.Items {
		for action, result := range item {
			if result.Status >= 400 && !(action == "delete" && result.Status == http.StatusNotFound) {
				return fmt.Errorf("opensearch bulk %s status %d: %v", action, result.Status, result.Error)
			}
		}
	}
	return fmt.Errorf("opensearch bulk reported item errors")
}

func (e *openSearchEngine) Count() (int, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	control, _, err := e.loadControl()
	if err != nil || control.ActiveIndex == "" {
		return 0, err
	}
	return e.countIndex(control.ActiveIndex)
}

func (e *openSearchEngine) countIndex(physicalIndex string) (int, error) {
	status, body, err := e.do(http.MethodPost, "/"+physicalIndex+"/_count", map[string]any{
		"query": map[string]any{"exists": map[string]any{"field": "object_id"}},
	})
	if err != nil {
		return 0, err
	}
	if status >= 400 {
		return 0, fmt.Errorf("opensearch count: %s", body)
	}
	var out struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

func (e *openSearchEngine) dropGeneration(name string) {
	if !strings.HasPrefix(name, e.prefix+"-g-") {
		return
	}
	_, _, _ = e.do(http.MethodDelete, "/"+name, nil)
}
