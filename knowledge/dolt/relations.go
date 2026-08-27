package dolt

import (
	"sort"

	"kc/kernel"
	"kc/knowledge"
)

func (r *Repository) LocateRelationObjectIDs(commit kernel.CommitID, endpoint knowledge.ObjectID, relationType, role string) ([]knowledge.ObjectID, error) {
	query := `SELECT DISTINCT TO_BASE64(e.relation_object_id) AS object_id64
        FROM kc_relation_endpoints AS OF ` + sqlString(string(commit)) + ` e
        JOIN kc_objects AS OF ` + sqlString(string(commit)) + ` o ON o.object_key=e.relation_key
        WHERE e.endpoint_key=` + sqlString(objectKey(endpoint)) + ` AND o.status='RESOLVED'`
	if relationType != "" {
		query += " AND o.relation_type=" + textSQL(relationType)
	}
	if role != "" {
		query += " AND e.role=" + textSQL(role)
	}
	query += " ORDER BY e.relation_key"
	rows, err := r.base.NativeQuery(query)
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
