package dolt

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/maintenance"
)

var (
	_ knowledge.UnitLocator           = (*Repository)(nil)
	_ knowledge.SchemaStore           = (*Repository)(nil)
	_ knowledge.BindingLocator        = (*Repository)(nil)
	_ knowledge.SchemaReferrerLocator = (*Repository)(nil)
)

// ObjectUnitPaths is the exact-read path list for Dataset prefix filters.
// Native tables store Canonical path_hint per unit; this does not list the tree.
func (r *Repository) ObjectUnitPaths(objectID knowledge.ObjectID, commit kernel.CommitID) ([]string, error) {
	if !r.HasCommit(commit) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	rows, err := r.base.NativeQuery("SELECT TO_BASE64(CAST(path_hint AS BINARY)) AS path64 FROM kc_units AS OF " + sqlString(string(commit)) + " WHERE object_key=" + sqlString(objectKey(objectID)) + " ORDER BY unit_key")
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	paths := make([]string, 0)
	for _, row := range rows {
		path, err := rowText64(row, "path64")
		if err != nil {
			return nil, err
		}
		if path == "" {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "knowledge unit has no storage path")
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths, nil
}

func (r *Repository) FileScopeObjectIDs(commit kernel.CommitID, prefixes, files []string, request maintenance.ScanRequest) (knowledge.ObjectIDPage, error) {
	limit, err := maintenance.NormalizeScanLimit(request.Limit)
	if err != nil {
		return knowledge.ObjectIDPage{}, err
	}
	if !r.HasCommit(commit) {
		return knowledge.ObjectIDPage{}, kernel.Fail(kernel.ErrVersionUnresolved, "scoped export commit is missing")
	}
	var clauses []string
	for _, prefix := range prefixes {
		prefix = strings.Trim(prefix, "/")
		if prefix == "" {
			clauses = append(clauses, "TRUE")
			continue
		}
		clauses = append(clauses, "(path_hint="+sqlString(prefix)+" OR LEFT(path_hint,"+strconv.Itoa(len([]rune(prefix))+1)+")="+sqlString(prefix+"/")+")")
	}
	for _, file := range files {
		clauses = append(clauses, "path_hint="+sqlString(strings.Trim(file, "/")))
	}
	if len(clauses) == 0 {
		return knowledge.ObjectIDPage{Exhausted: true}, nil
	}
	rows, err := r.base.NativeQuery("SELECT DISTINCT object_key, TO_BASE64(CAST(object_id AS BINARY)) AS object_id64 FROM kc_units AS OF " + sqlString(string(commit)) + " WHERE (" + strings.Join(clauses, " OR ") + ") AND object_key>" + sqlString(request.Continuation) + " ORDER BY object_key LIMIT " + strconv.Itoa(limit+1))
	if err != nil {
		return knowledge.ObjectIDPage{}, err
	}
	page := knowledge.ObjectIDPage{Exhausted: len(rows) <= limit}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	for _, row := range rows {
		id, err := rowText64(row, "object_id64")
		if err != nil {
			return knowledge.ObjectIDPage{}, err
		}
		page.ObjectIDs = append(page.ObjectIDs, knowledge.ObjectID(id))
		page.Continuation = rowString(row, "object_key")
	}
	if page.Exhausted {
		page.Continuation = ""
	}
	return page, nil
}

func (r *Repository) SchemaObjectIDs(commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	rows, err := r.base.NativeQuery(`SELECT TO_BASE64(CAST(object_id AS BINARY)) AS object_id64
        FROM kc_objects AS OF ` + sqlString(string(commit)) + `
        WHERE is_schema=TRUE AND status='RESOLVED' ORDER BY object_key`)
	if err != nil {
		return nil, err
	}
	ids := make([]knowledge.ObjectID, 0, len(rows))
	for _, row := range rows {
		value, err := rowText64(row, "object_id64")
		if err != nil {
			return nil, err
		}
		ids = append(ids, knowledge.ObjectID(value))
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

// BindingSchemaObjectIDs locates the bounded declaration namespace through
// native tables. It is planning metadata, not a consumer Snapshot scan.
func (r *Repository) BindingSchemaObjectIDs(commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	ids, err := r.SchemaObjectIDs(commit)
	if err != nil {
		return nil, err
	}
	seen := map[knowledge.ObjectID]struct{}{}
	for _, id := range ids {
		value, readErr := r.Read(id, commit)
		if readErr != nil {
			return nil, readErr
		}
		definition, parseErr := knowledge.ParseSchemaDefinition(id, value.Value)
		if parseErr != nil {
			return nil, parseErr
		}
		if definition.Bound() {
			seen[id] = struct{}{}
		}
	}
	rows, err := r.base.NativeQuery(`SELECT TO_BASE64(CAST(schema_ref AS BINARY)) AS schema_ref64, value_source_json
        FROM kc_units AS OF ` + sqlString(string(commit)) + `
        WHERE value_source_json IS NOT NULL AND schema_ref <> '' ORDER BY unit_key`)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		var source knowledge.ValueSource
		if err := json.Unmarshal([]byte(rowString(row, "value_source_json")), &source); err != nil {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid native value_source_json: %v", err)
		}
		if source.Kind != knowledge.ValueSourceBinding {
			continue
		}
		ref, err := rowText64(row, "schema_ref64")
		if err != nil {
			return nil, err
		}
		if parsed, ok := knowledge.ParseSchemaRef(ref); ok {
			seen[parsed.Object] = struct{}{}
		}
	}
	out := make([]knowledge.ObjectID, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// SchemaReferrerAddresses answers the reverse schema_ref question from the
// indexed schema_object_key column, so publishing a Domain Schema stays bounded
// by the number of referencing units rather than the size of the Repository.
func (r *Repository) SchemaReferrerAddresses(schema knowledge.ObjectID, commit kernel.CommitID) ([]knowledge.Address, error) {
	rows, err := r.base.NativeQuery(`SELECT TO_BASE64(CAST(object_id AS BINARY)) AS object_id64,
        kind, TO_BASE64(CAST(aspect_name AS BINARY)) AS aspect_name64,
        TO_BASE64(CAST(member_key AS BINARY)) AS member_key64,
        TO_BASE64(CAST(schema_ref AS BINARY)) AS schema_ref64
        FROM kc_units AS OF ` + sqlString(string(commit)) + `
        WHERE schema_object_key=` + sqlString(objectKey(schema)) + ` ORDER BY unit_key`)
	if err != nil {
		return nil, err
	}
	addresses := make([]knowledge.Address, 0, len(rows))
	for _, row := range rows {
		objectID, err := rowText64(row, "object_id64")
		if err != nil {
			return nil, err
		}
		// The index key is a digest; confirm the full identity before trusting it.
		ref, err := rowText64(row, "schema_ref64")
		if err != nil {
			return nil, err
		}
		parsed, ok := knowledge.ParseSchemaRef(ref)
		if !ok || parsed.Object != schema {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed,
				"native schema key collision for %s on unit %s", schema, objectID)
		}
		if knowledge.IsSchemaObject(knowledge.ObjectID(objectID)) {
			continue
		}
		aspectName, err := rowText64(row, "aspect_name64")
		if err != nil {
			return nil, err
		}
		memberKey, err := rowText64(row, "member_key64")
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, knowledge.InferAddress(
			knowledge.ObjectID(objectID), aspectName, memberKey, rowString(row, "kind")))
	}
	sort.Slice(addresses, func(i, j int) bool {
		return knowledge.AddressKey(addresses[i]) < knowledge.AddressKey(addresses[j])
	})
	return addresses, nil
}
