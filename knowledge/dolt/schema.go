package dolt

import (
	"sort"

	"kc/kernel"
	"kc/knowledge"
)

func (r *Repository) SchemaObjectIDs(commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	rows, err := r.base.NativeQuery(`SELECT TO_BASE64(object_id) AS object_id64
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
