package dolt

import (
	"encoding/base64"
	"encoding/json"
	"strconv"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	knowledgemaintenance "kc/knowledge/maintenance"
	"kc/knowledge/unitcodec"
)

type objectManifest struct {
	ObjectID          knowledge.ObjectID
	Kind              knowledge.AddressKind
	Status            knowledge.ResolutionStatus
	ObjectDigest      kernel.Digest
	DeclarationDigest kernel.Digest
}

func (r *Repository) manifest(objectID knowledge.ObjectID, commit kernel.CommitID) (objectManifest, bool, error) {
	if !r.HasCommit(commit) {
		return objectManifest{}, false, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	rows, err := r.base.NativeQuery(`SELECT kind, status, object_digest, declaration_digest,
        TO_BASE64(object_id) AS object_id64 FROM kc_objects AS OF ` + sqlString(string(commit)) +
		" WHERE object_key=" + sqlString(objectKey(objectID)) + " LIMIT 1")
	if err != nil {
		return objectManifest{}, false, err
	}
	if len(rows) == 0 {
		return objectManifest{}, false, nil
	}
	rawID, err := rowText64(rows[0], "object_id64")
	if err != nil {
		return objectManifest{}, false, err
	}
	if rawID != string(objectID) {
		return objectManifest{}, false, kernel.Fail(kernel.ErrPreconditionFailed, "native object key collision for %s", objectID)
	}
	return objectManifest{
		ObjectID: objectID, Kind: knowledge.AddressKind(rowString(rows[0], "kind")),
		Status:            knowledge.ResolutionStatus(rowString(rows[0], "status")),
		ObjectDigest:      kernel.Digest(rowString(rows[0], "object_digest")),
		DeclarationDigest: kernel.Digest(rowString(rows[0], "declaration_digest")),
	}, true, nil
}

func (r *Repository) loadUnits(objectIDs []knowledge.ObjectID, commit kernel.CommitID) (map[knowledge.ObjectID][]unitcodec.Unit, error) {
	out := map[knowledge.ObjectID][]unitcodec.Unit{}
	if len(objectIDs) == 0 {
		return out, nil
	}
	keys := make([]string, 0, len(objectIDs))
	wanted := map[string]knowledge.ObjectID{}
	for _, objectID := range objectIDs {
		key := objectKey(objectID)
		keys = append(keys, sqlString(key))
		wanted[key] = objectID
	}
	rows, err := r.base.NativeQuery(unitSelect + " AS OF " + sqlString(string(commit)) +
		" WHERE object_key IN (" + joinComma(keys) + ") ORDER BY object_key, unit_key")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		key := rowString(row, "object_key")
		objectID, ok := wanted[key]
		if !ok {
			continue
		}
		unit, err := decodeUnit(row)
		if err != nil {
			return nil, err
		}
		if unit.Address.ObjectID != objectID {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "native object key collision for %s", objectID)
		}
		out[objectID] = append(out[objectID], unit)
	}
	return out, nil
}

func joinComma(values []string) string {
	result := ""
	for i, value := range values {
		if i > 0 {
			result += ","
		}
		result += value
	}
	return result
}

func (r *Repository) ReadMany(objectIDs []knowledge.ObjectID, commit kernel.CommitID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	units, err := r.loadUnits(objectIDs, commit)
	if err != nil {
		return nil, err
	}
	out := map[knowledge.ObjectID]knowledge.KnowledgeValue{}
	for objectID, objectUnits := range units {
		if len(objectUnits) == 0 {
			continue
		}
		value, err := unitcodec.AssembleKnowledgeValue(r.ID(), objectID, commit, objectUnits)
		if err != nil {
			return nil, err
		}
		out[objectID] = value
	}
	return out, nil
}

func (r *Repository) Read(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	manifest, ok, err := r.manifest(objectID, commit)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	if !ok || manifest.Status != knowledge.StatusResolved {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commit)
	}
	values, err := r.ReadMany([]knowledge.ObjectID{objectID}, commit)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	value, ok := values[objectID]
	if !ok {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "native manifest for %s has no units", objectID)
	}
	return value, nil
}

func (r *Repository) Resolve(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.Resolution, error) {
	manifest, ok, err := r.manifest(objectID, commit)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	if !ok {
		return knowledge.Resolution{Repository: r.ID(), Commit: commit, ObjectID: objectID,
			Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID}, Status: knowledge.StatusUnresolved}, nil
	}
	resolution := knowledge.Resolution{Repository: r.ID(), Commit: commit, ObjectID: objectID,
		Address: knowledge.Address{Kind: manifest.Kind, ObjectID: objectID}, Status: manifest.Status,
		Digest: manifest.ObjectDigest, DeclarationDigest: manifest.DeclarationDigest}
	if manifest.Status != knowledge.StatusResolved {
		return resolution, nil
	}
	units, err := r.loadUnits([]knowledge.ObjectID{objectID}, commit)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	if len(units[objectID]) > 0 {
		resolution.PathHint = repofile.EntityPathHint(units[objectID], objectID)
	}
	if len(units[objectID]) == 1 {
		resolution.SchemaRef = units[objectID][0].SchemaRef
		resolution.ValueSource = units[objectID][0].ValueSource
	}
	return resolution, nil
}

func (r *Repository) unitAt(address knowledge.Address, commit kernel.CommitID) (unitcodec.Unit, bool, error) {
	rows, err := r.base.NativeQuery(unitSelect + " AS OF " + sqlString(string(commit)) +
		" WHERE unit_key=" + sqlString(unitKey(address)) + " LIMIT 1")
	if err != nil {
		return unitcodec.Unit{}, false, err
	}
	if len(rows) == 0 {
		return unitcodec.Unit{}, false, nil
	}
	unit, err := decodeUnit(rows[0])
	if err != nil {
		return unitcodec.Unit{}, false, err
	}
	if knowledge.AddressKey(unit.Address) != knowledge.AddressKey(address) {
		return unitcodec.Unit{}, false, kernel.Fail(kernel.ErrPreconditionFailed, "native unit key collision for %s", knowledge.AddressKey(address))
	}
	return unit, true, nil
}

func (r *Repository) ResolveAddress(address knowledge.Address, commit kernel.CommitID) (knowledge.Resolution, error) {
	if err := knowledge.AssertWritable(address); err != nil {
		return knowledge.Resolution{}, err
	}
	unit, ok, err := r.unitAt(address, commit)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	if !ok {
		return knowledge.Resolution{Repository: r.ID(), Commit: commit, ObjectID: address.ObjectID,
			Address: address, Status: knowledge.StatusUnresolved}, nil
	}
	hint := unit.PathHint
	if hint == "" {
		hint = unit.Path
	}
	return knowledge.Resolution{Repository: r.ID(), Commit: commit, ObjectID: address.ObjectID,
		Address: address, Status: knowledge.StatusResolved, PathHint: hint, Digest: unit.Digest,
		DeclarationDigest: knowledge.DeclarationDigest(unit.SchemaRef, unit.ValueSource),
		SchemaRef:         unit.SchemaRef, ValueSource: unit.ValueSource}, nil
}

func (r *Repository) ReadAddress(address knowledge.Address, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	unit, ok, err := r.unitAt(address, commit)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	if !ok {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "address %s is missing at commit %s", knowledge.AddressKey(address), commit)
	}
	return knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: r.ID(), Object: address.ObjectID},
		Repository:   r.ID(), Commit: commit, Address: address, Value: unit.Value, Provenance: unit.Provenance,
		Declarations: []knowledge.UnitDeclaration{repofile.DeclarationOf(unit)},
	}, nil
}

func (r *Repository) GetProvenance(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.ProvenanceTrace, error) {
	units, err := r.loadUnits([]knowledge.ObjectID{objectID}, commit)
	if err != nil {
		return knowledge.ProvenanceTrace{}, err
	}
	if len(units[objectID]) == 0 {
		return knowledge.ProvenanceTrace{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commit)
	}
	chain := []knowledge.ProvenanceEnvelope{}
	for _, unit := range units[objectID] {
		if unit.Provenance != nil {
			chain = append(chain, *unit.Provenance)
		}
	}
	return knowledge.ProvenanceTrace{Repository: r.ID(), Commit: commit, ObjectID: objectID, Chain: chain}, nil
}

type pageState struct {
	Version   int             `json:"version"`
	Commit    kernel.CommitID `json:"commit"`
	After     string          `json:"after"`
	Signature kernel.Digest   `json:"signature"`
}

func encodePageState(state pageState) string {
	state.Version = 1
	state.Signature = ""
	state.Signature = kernel.CanonicalDigest(state)
	raw, _ := json.Marshal(state)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodePageState(token string, commit kernel.CommitID) (pageState, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return pageState{}, kernel.Fail(kernel.ErrPreconditionFailed, "invalid native Dolt continuation")
	}
	state := pageState{}
	if json.Unmarshal(raw, &state) != nil {
		return pageState{}, kernel.Fail(kernel.ErrPreconditionFailed, "invalid native Dolt continuation")
	}
	signature := state.Signature
	state.Signature = ""
	if state.Version != 1 || state.Commit != commit || signature != kernel.CanonicalDigest(state) {
		return pageState{}, kernel.Fail(kernel.ErrPreconditionFailed, "native Dolt continuation does not match commit %s", commit)
	}
	return state, nil
}

func (r *Repository) ScanSnapshotPage(commit kernel.CommitID, request knowledgemaintenance.ScanRequest) (knowledgemaintenance.ScanPage, error) {
	limit, err := knowledgemaintenance.NormalizeScanLimit(request.Limit)
	if err != nil {
		return knowledgemaintenance.ScanPage{}, err
	}
	after := ""
	if request.Continuation != "" {
		state, err := decodePageState(request.Continuation, commit)
		if err != nil {
			return knowledgemaintenance.ScanPage{}, err
		}
		after = state.After
	}
	query := `SELECT object_key, TO_BASE64(object_id) AS object_id64 FROM kc_objects AS OF ` + sqlString(string(commit)) +
		` WHERE status='RESOLVED' AND object_key>` + sqlString(after) + ` ORDER BY object_key LIMIT ` + strconv.Itoa(limit+1)
	rows, err := r.base.NativeQuery(query)
	if err != nil {
		return knowledgemaintenance.ScanPage{}, err
	}
	exhausted := len(rows) <= limit
	if len(rows) > limit {
		rows = rows[:limit]
	}
	ids := make([]knowledge.ObjectID, 0, len(rows))
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		rawID, err := rowText64(row, "object_id64")
		if err != nil {
			return knowledgemaintenance.ScanPage{}, err
		}
		ids = append(ids, knowledge.ObjectID(rawID))
		keys = append(keys, rowString(row, "object_key"))
	}
	values, err := r.ReadMany(ids, commit)
	if err != nil {
		return knowledgemaintenance.ScanPage{}, err
	}
	page := knowledgemaintenance.ScanPage{Values: make([]knowledge.KnowledgeValue, 0, len(ids)), Exhausted: exhausted}
	for _, id := range ids {
		value, ok := values[id]
		if !ok {
			return knowledgemaintenance.ScanPage{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "native manifest for %s has no units", id)
		}
		page.Values = append(page.Values, value)
	}
	if !exhausted && len(keys) > 0 {
		page.Continuation = encodePageState(pageState{Commit: commit, After: keys[len(keys)-1]})
	}
	return page, nil
}
