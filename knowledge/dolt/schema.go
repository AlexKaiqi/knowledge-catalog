package dolt

import (
	"encoding/json"
	"sort"

	"kc/kernel"
	"kc/knowledge"
)

var (
	_ knowledge.SchemaStore    = (*Repository)(nil)
	_ knowledge.BindingLocator = (*Repository)(nil)
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
