package opensearchimpl

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

type controlDoc struct {
	ScopeDemand          *scopeDemand               `json:"scope_demand,omitempty"`
	QueryLeaseVersion    int                        `json:"query_lease_version,omitempty"`
	ObservationDeadlines map[string]int64           `json:"observation_deadlines,omitempty"`
	CursorKey            []byte                     `json:"cursor_key,omitempty"`
	PublicationVersion   int                        `json:"publication_version,omitempty"`
	PublicationTicket    string                     `json:"publication_ticket,omitempty"`
	PublicationEvidence  []byte                     `json:"publication_evidence,omitempty"`
	RecoveryAfter        string                     `json:"recovery_after,omitempty"`
	Resource             *generationResource        `json:"resource,omitempty"`
	Repository           string                     `json:"repository"`
	ActiveIndex          string                     `json:"active_index"`
	Generation           string                     `json:"generation"`
	ProjectionRevision   string                     `json:"projection_revision,omitempty"`
	State                string                     `json:"state"`
	Basis                string                     `json:"basis"`
	AccessDigest         string                     `json:"access_digest"`
	ObservationDigest    string                     `json:"observation_digest,omitempty"`
	PhysicalDigest       string                     `json:"physical_digest"`
	ProviderRevision     string                     `json:"provider_revision"`
	Mode                 string                     `json:"mode"`
	Cause                string                     `json:"cause"`
	Coverage             float64                    `json:"coverage"`
	ObjectCount          int                        `json:"object_count"`
	SchemaAdoptions      []retrieval.SchemaAdoption `json:"schema_adoptions,omitempty"`
	LastError            string                     `json:"last_error"`
}

type controlVersion struct {
	SeqNo       int64
	PrimaryTerm int64
}

type osDoc struct {
	SourceRepository  string               `json:"source_repository,omitempty"`
	SourceCommit      string               `json:"source_commit,omitempty"`
	ObjectID          string               `json:"object_id"`
	Kind              string               `json:"kind"`
	EligibleFields    []string             `json:"eligible_fields"`
	AllText           string               `json:"all_text"`
	Cells             []osCell             `json:"cells"`
	RelationType      string               `json:"relation_type"`
	RelationDirection string               `json:"relation_direction"`
	RelationEndpoints []osRelationEndpoint `json:"relation_endpoints"`
	ObjectDigest      string               `json:"object_digest"`
	SemanticVectors   []osSemanticVector   `json:"semantic_vectors,omitempty"`
}

type osCell struct {
	Field        string              `json:"field"`
	Access       []reader.AccessHint `json:"access"`
	StringValue  *string             `json:"string_value,omitempty"`
	TextValue    string              `json:"text_value,omitempty"`
	LongValue    *int64              `json:"long_value,omitempty"`
	DoubleValue  *float64            `json:"double_value,omitempty"`
	BooleanValue *bool               `json:"boolean_value,omitempty"`
	DateValue    string              `json:"date_value,omitempty"`
}

type osRelationEndpoint struct {
	Role     string `json:"role"`
	ObjectID string `json:"object_id"`
}

type osSemanticVector struct {
	Member string    `json:"member"`
	Model  string    `json:"model"`
	Vector []float32 `json:"vector"`
}

func encodeDoc(doc index.CompiledDoc, vectors [][]float32) (osDoc, error) {
	out := osDoc{
		SourceRepository: string(doc.SourceRepository), SourceCommit: string(doc.SourceCommit),
		ObjectID: string(doc.ObjectID), Kind: string(doc.Kind), EligibleFields: doc.EligibleFields,
		AllText: doc.Text, ObjectDigest: string(doc.ObjectDigest), Cells: make([]osCell, 0, len(doc.Cells)),
		RelationEndpoints: []osRelationEndpoint{},
	}
	for _, cell := range doc.Cells {
		dateKey := ""
		if cell.DateValue != "" {
			var err error
			dateKey, err = encodeTemporalKey(cell.DateValue)
			if err != nil {
				return osDoc{}, err
			}
		}
		out.Cells = append(out.Cells, osCell{
			Field: cell.Field, Access: cell.Access, StringValue: cell.StringValue, TextValue: cell.TextValue,
			LongValue: cell.LongValue, DoubleValue: cell.DoubleValue,
			BooleanValue: cell.BooleanValue, DateValue: dateKey,
		})
	}
	if len(vectors) != len(doc.Semantic) {
		return osDoc{}, kernel.Fail(kernel.ErrPreconditionFailed, "semantic vector count does not match declared member inputs")
	}
	for i, field := range doc.Semantic {
		out.SemanticVectors = append(out.SemanticVectors, osSemanticVector{Member: field.Member, Model: field.Model, Vector: vectors[i]})
	}
	if doc.Relation != nil {
		out.RelationType = doc.Relation.Type
		out.RelationDirection = string(doc.Relation.Direction)
		for _, endpoint := range doc.Relation.Endpoints {
			out.RelationEndpoints = append(out.RelationEndpoints, osRelationEndpoint{
				Role: endpoint.Role, ObjectID: string(endpoint.Object),
			})
		}
	}
	return out, nil
}

// Resource records preserve READY meaning under lifecycle bookkeeping states.
func readyResourceMeta(record controlDoc) index.Meta {
	record.State = index.ProjectionStateReady
	return metaFromControl(record)
}

func metaFromControl(control controlDoc) index.Meta {
	return index.Meta{
		Basis: kernel.CommitID(control.Basis), AccessDigest: kernel.Digest(control.AccessDigest),
		ObservationDigest: kernel.Digest(control.ObservationDigest),
		PhysicalDigest:    kernel.Digest(control.PhysicalDigest), ProviderRevision: control.ProviderRevision,
		Generation: control.Generation, State: control.State, Coverage: control.Coverage,
		Revision: control.ProjectionRevision,
		Mode:     control.Mode, Cause: control.Cause,
		SchemaAdoptions: control.SchemaAdoptions,
	}
}

func controlFromMeta(repository kernel.RepositoryID, active, generation, state string, meta index.Meta, count int) controlDoc {
	// The wire omits an empty adoption list. Keep the in-memory intent in the
	// same form so authoritative readback can prove a lost publication ack.
	if len(meta.SchemaAdoptions) == 0 {
		meta.SchemaAdoptions = nil
	}
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
		SchemaAdoptions: meta.SchemaAdoptions,
	}
}

func (e *openSearchEngine) loadControl() (controlDoc, *controlVersion, error) {
	return e.loadControlContext(context.Background())
}

func (e *openSearchEngine) loadControlContext(ctx context.Context) (controlDoc, *controlVersion, error) {
	if len(e.fixedControl) > 0 {
		if e.queryProtection != nil {
			if err := e.queryProtection.status(); err != nil {
				return controlDoc{}, nil, err
			}
		}
		if err := ctx.Err(); err != nil {
			return controlDoc{}, nil, err
		}
		var control controlDoc
		if err := json.Unmarshal(e.fixedControl, &control); err != nil {
			return controlDoc{}, nil, err
		}
		version := e.fixedVersion
		return control, &version, nil
	}
	return e.loadControlRecord(ctx, e.controlID)
}

func (e *openSearchEngine) loadControlRecord(ctx context.Context, id string) (controlDoc, *controlVersion, error) {
	status, body, err := e.readControlResponse(ctx, "/"+e.controlIndexPath()+"/_doc/"+url.PathEscape(id))
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
	return e.putControlRecord(e.controlID, control, expected)
}

func (e *openSearchEngine) putControlRecord(id string, control controlDoc, expected *controlVersion) (*controlVersion, error) {
	return e.putControlRecordContext(context.Background(), id, control, expected)
}

func (e *openSearchEngine) putControlRecordContext(ctx context.Context, id string, control controlDoc, expected *controlVersion) (*controlVersion, error) {
	path := "/" + e.controlIndexPath() + "/_doc/" + url.PathEscape(id) + "?refresh=true"
	if expected == nil {
		path += "&op_type=create"
	} else {
		path += "&if_seq_no=" + strconv.FormatInt(expected.SeqNo, 10) + "&if_primary_term=" + strconv.FormatInt(expected.PrimaryTerm, 10)
	}
	status, body, err := e.doContext(ctx, http.MethodPut, path, control)
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
	return e.LoadMetaContext(context.Background())
}

func (e *openSearchEngine) LoadMetaContext(ctx context.Context) (index.Meta, error) {
	if err := e.readLockContext(ctx); err != nil {
		return index.Meta{}, err
	}
	defer e.mu.RUnlock()
	control, _, err := e.loadControlContext(ctx)
	if err != nil || control.ActiveIndex == "" {
		return index.Meta{}, err
	}
	status, _, err := e.doContext(ctx, http.MethodHead, "/"+url.PathEscape(control.ActiveIndex), nil)
	if err != nil {
		return index.Meta{}, err
	}
	if status == http.StatusNotFound {
		return index.Meta{}, nil
	}
	if status >= http.StatusMultipleChoices {
		return index.Meta{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "opensearch projection generation readiness returned HTTP %d", status)
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
	engine              *openSearchEngine
	old                 controlDoc
	buildVersion        *controlVersion
	physicalIndex       string
	generation          string
	meta                index.Meta
	expected            int
	done                bool
	cold                bool
	publicationRejected bool
	resourceContext     context.Context
	preparation         *preparedResource
}

func (e *openSearchEngine) BeginRebuild(meta index.Meta) (index.RebuildSession, error) {
	if err := e.requireMutableProjection(); err != nil {
		return nil, err
	}
	e.buildMu.Lock()
	// The control index must exist under its declared mapping before demand is
	// recorded: a demand write to a missing index lets OpenSearch infer the
	// mapping dynamically, which then rejects the declared one.
	if err := e.ensureControlIndex(); err != nil {
		e.buildMu.Unlock()
		return nil, err
	}
	if err := e.enterDemand(context.Background()); err != nil {
		e.buildMu.Unlock()
		return nil, err
	}
	if err := e.reconcileCandidates(); err != nil {
		e.buildMu.Unlock()
		return nil, err
	}
	e.mu.RLock()
	old, version, err := e.loadControl()
	e.mu.RUnlock()
	if err != nil {
		e.buildMu.Unlock()
		return nil, err
	}
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		e.buildMu.Unlock()
		return nil, err
	}
	generation := hex.EncodeToString(identity[:])
	physicalIndex := e.prefix + "-g-" + generation
	session := &rebuildSession{engine: e, old: old, buildVersion: version, physicalIndex: physicalIndex,
		generation: generation, meta: meta, cold: old.ActiveIndex == ""}
	if err := session.registerResource(); err != nil {
		e.buildMu.Unlock()
		return nil, err
	}
	if err := session.resourceIO(func() error { return e.createGeneration(physicalIndex) }); err != nil {
		return nil, session.Abort(err)
	}
	if session.cold {
		building := controlFromMeta(e.repository, "", generation, index.ProjectionStateBuilding, meta, 0)
		session.buildVersion, err = e.putControl(building, version)
		if err != nil {
			return nil, session.Abort(err)
		}
	}
	return session, nil
}

func (s *rebuildSession) Append(docs []index.CompiledDoc) error {
	if s.done {
		return kernel.Fail(kernel.ErrPreconditionFailed, "OpenSearch rebuild session is closed")
	}
	if len(docs) == 0 {
		return nil
	}
	if _, err := s.engine.bulkGuarded(s.physicalIndex, docs, nil, false, s.resourceIO); err != nil {
		return err
	}
	s.expected += len(docs)
	return nil
}

func (s *rebuildSession) Commit() error {
	if s.done {
		return kernel.Fail(kernel.ErrPreconditionFailed, "OpenSearch rebuild session is closed")
	}
	if err := s.resourceIO(func() error { return s.engine.refresh(s.physicalIndex) }); err != nil {
		return s.Abort(err)
	}
	count, err := s.engine.countIndex(s.physicalIndex)
	if err != nil {
		return s.Abort(err)
	}
	if count != s.expected {
		return s.Abort(fmt.Errorf("opensearch generation count %d does not match compiled count %d", count, s.expected))
	}
	if err := s.preservePreviousGeneration(); err != nil {
		return s.Abort(err)
	}
	if err := s.resourcePhase(resourcePublishing); err != nil {
		return s.Abort(err)
	}
	ready := controlFromMeta(s.engine.repository, s.physicalIndex, s.generation, index.ProjectionStateReady, s.meta, count)
	ready.QueryLeaseVersion = 1
	ready = withObservationDeadlines(s.old, ready, time.Now())
	if s.preparation != nil {
		s.preparation.ready = ready
	}
	s.engine.mu.Lock()
	_, publishErr := s.engine.putControl(ready, s.buildVersion)
	s.engine.mu.Unlock()
	if publishErr != nil {
		if kernel.CodeOf(publishErr) == kernel.ErrNonFastForward {
			s.publicationRejected = true
			return s.Abort(publishErr)
		}
		if err := s.recoverPublication(ready, publishErr); err != nil {
			return err
		}
	}
	protectErr := s.resourcePhase(resourceAdopted)
	s.done = true
	s.engine.buildMu.Unlock()
	return protectErr
}

func (s *rebuildSession) Abort(buildErr error) error {
	if s.done {
		return buildErr
	}
	if s.cold {
		failed := controlFromMeta(s.engine.repository, "", s.generation, index.ProjectionStateFailed, s.meta, 0)
		failed.LastError = buildErr.Error()
		_, _ = s.engine.putControl(failed, s.buildVersion)
	}
	if err := s.discardResource(); err != nil {
		buildErr = errors.Join(kernel.Fail(kernel.ErrTemporaryUnavailable, "candidate %s retained for recovery: %v", s.physicalIndex, err), buildErr)
	}
	s.done = true
	s.engine.buildMu.Unlock()
	return buildErr
}

func (e *openSearchEngine) Apply(upserts []index.CompiledDoc, deletes []objectmodel.ObjectID, meta index.Meta) error {
	if err := e.requireMutableProjection(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	control, version, err := e.loadControl()
	if err != nil {
		return err
	}
	if control.ActiveIndex == "" || control.State != index.ProjectionStateReady {
		return kernel.Fail(kernel.ErrPreconditionFailed, "opensearch projection is not READY")
	}
	return e.updateResource(control, func(guard func(func() error) error) error {
		return e.applyGeneration(control, version, upserts, deletes, meta, guard)
	})
}

func (e *openSearchEngine) applyGeneration(control controlDoc, version *controlVersion, upserts []index.CompiledDoc, deletes []objectmodel.ObjectID, meta index.Meta, guard func(func() error) error) error {
	updating := control
	updating.State = index.ProjectionStateUpdating
	updating.LastError = ""
	updateVersion, err := e.putControl(updating, version)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrNonFastForward {
			return completedPhysical(err)
		}
		return err
	}
	fail := func(updateErr error) error {
		failed := updating
		failed.State = index.ProjectionStateFailed
		failed.LastError = updateErr.Error()
		_, _ = e.putControl(failed, updateVersion)
		return updateErr
	}
	delta, err := e.bulkGuarded(control.ActiveIndex, upserts, deletes, true, guard)
	if err != nil {
		return fail(err)
	}
	count := control.ObjectCount + delta
	ready := controlFromMeta(e.repository, control.ActiveIndex, control.Generation, index.ProjectionStateReady, meta, count)
	ready.QueryLeaseVersion = control.QueryLeaseVersion
	ready = withObservationDeadlines(control, ready, time.Now())
	if _, err := e.putControl(ready, updateVersion); err != nil {
		return fail(err)
	}
	return nil
}

func (e *openSearchEngine) bulkGuarded(physicalIndex string, docs []index.CompiledDoc, deletes []objectmodel.ObjectID, waitForRefresh bool, guard func(func() error) error) (int, error) {
	const batchSize = 500
	delta := 0
	for start := 0; start < len(deletes); start += batchSize {
		end := start + batchSize
		if end > len(deletes) {
			end = len(deletes)
		}
		var body bytes.Buffer
		for _, id := range deletes[start:end] {
			writeNDJSON(&body, map[string]any{"delete": map[string]any{"_index": physicalIndex, "_id": documentID(string(id))}})
		}
		batchDelta, err := e.sendBulkGuarded(body.Bytes(), waitForRefresh, guard)
		if err != nil {
			return 0, err
		}
		delta += batchDelta
	}
	for start := 0; start < len(docs); start += batchSize {
		end := start + batchSize
		if end > len(docs) {
			end = len(docs)
		}
		var body bytes.Buffer
		vectors, err := e.embedBatchWithCache(docs[start:end], guard)
		if err != nil {
			return 0, err
		}
		for batchIndex, doc := range docs[start:end] {
			encoded, err := encodeDoc(doc, vectors[batchIndex])
			if err != nil {
				return 0, err
			}
			writeNDJSON(&body, map[string]any{"index": map[string]any{"_index": physicalIndex, "_id": documentID(string(doc.ObjectID))}})
			writeNDJSON(&body, encoded)
		}
		// refresh=wait_for covers only shards touched by this bulk request.
		// Every incremental batch must pass its own visibility barrier before READY.
		batchDelta, err := e.sendBulkGuarded(body.Bytes(), waitForRefresh, guard)
		if err != nil {
			return 0, err
		}
		delta += batchDelta
	}
	return delta, nil
}

func writeNDJSON(buffer *bytes.Buffer, value any) {
	encoded, _ := json.Marshal(value)
	buffer.Write(encoded)
	buffer.WriteByte('\n')
}

func (e *openSearchEngine) sendBulk(payload []byte, waitForRefresh bool) (int, error) {
	if len(payload) == 0 {
		return 0, nil
	}
	path := "/_bulk"
	if waitForRefresh {
		path += "?refresh=wait_for"
	}
	status, body, err := e.doBytes(http.MethodPost, path, payload, "application/x-ndjson")
	if err != nil {
		return 0, err
	}
	if status >= 400 {
		return 0, completedPhysical(fmt.Errorf("opensearch bulk: %s", body))
	}
	var response struct {
		Errors bool `json:"errors"`
		Items  []map[string]struct {
			Status int            `json:"status"`
			Result string         `json:"result"`
			Error  map[string]any `json:"error"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return 0, err
	}
	delta := 0
	var failure error
	definitive := true
	for _, item := range response.Items {
		for action, result := range item {
			if result.Status >= 400 && !(action == "delete" && result.Status == http.StatusNotFound) {
				failure = errors.Join(failure, fmt.Errorf("opensearch bulk %s status %d: %v", action, result.Status, result.Error))
				switch result.Status {
				case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict:
				default:
					definitive = false
				}
			}
			if action == "index" && (result.Result == "created" || result.Status == http.StatusCreated) {
				delta++
			}
			if action == "delete" && result.Result == "deleted" {
				delta--
			}
		}
	}
	if failure != nil {
		if definitive {
			failure = completedPhysical(failure)
		}
		return 0, failure
	}
	return delta, nil
}

func (e *openSearchEngine) Count() (int, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	control, _, err := e.loadControl()
	if err != nil || control.ActiveIndex == "" {
		return 0, err
	}
	return control.ObjectCount, nil
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
		Count  int            `json:"count"`
		Shards *shardResponse `json:"_shards"`
	}
	if err := decodeJSON(body, &out); err != nil {
		return 0, err
	}
	if err := out.Shards.check("count"); err != nil {
		return 0, err
	}
	return out.Count, nil
}
