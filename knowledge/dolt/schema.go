package dolt

import (
	"encoding/json"
	"sort"

	"kc/kernel"
	"kc/knowledge"
)

var (
	_ knowledge.SchemaStore           = (*Repository)(nil)
	_ knowledge.BindingLocator        = (*Repository)(nil)
	_ knowledge.SchemaReferrerLocator = (*Repository)(nil)
)

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
	rows, err := r.base.NativeQuery(`SELECT TO_BASE64(CAST(schema_ref AS BINARY)) AS schema_ref64, value_source_json
        FROM kc_units AS OF ` + sqlString(string(commit)) + `
        WHERE value_source_json IS NOT NULL AND schema_ref <> '' ORDER BY unit_key`)
	if err != nil {
		return nil, err
	}
	seen := map[knowledge.ObjectID]struct{}{}
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
	ids := make([]knowledge.ObjectID, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
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
