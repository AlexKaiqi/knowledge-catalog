package local

import (
	"database/sql"
	"strings"

	"kc/kernel"
	"kc/reader"
)

func clauseIDs(db *sql.DB, c reader.SearchClause, spec reader.IndexSpec) ([]kernel.ObjectID, error) {
	switch c.Op {
	case reader.OpMatch:
		if c.Path == "" {
			return queryFTS(db, c.Value)
		}
		return queryMatchPath(db, c.Path, c.Value)
	case reader.OpEQ:
		return queryFilter(db, c.Path, c.Value)
	case reader.OpIN:
		return queryIN(db, c.Path, c.Values)
	case reader.OpNEQ:
		return queryNEQ(db, c.Path, c.Value)
	case reader.OpExists:
		return queryExists(db, c.Path)
	case reader.OpGT, reader.OpGTE, reader.OpLT, reader.OpLTE:
		field, _ := spec.Field(c.Path)
		return queryCompare(db, c, reader.NumericType(field.Type))
	default:
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown search operator %s", c.Op)
	}
}

func queryIN(db *sql.DB, path string, values []string) ([]kernel.ObjectID, error) {
	if len(values) == 0 {
		return nil, nil
	}
	args := make([]any, 0, 1+len(values))
	args = append(args, path)
	placeholders := make([]string, len(values))
	for i, v := range values {
		placeholders[i] = "?"
		args = append(args, v)
	}
	rows, err := db.Query(`SELECT object_id FROM fields WHERE path = ? AND value IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func queryNEQ(db *sql.DB, path, value string) ([]kernel.ObjectID, error) {
	rows, err := db.Query(`
		SELECT object_id FROM objects
		WHERE object_id NOT IN (SELECT object_id FROM fields WHERE path = ? AND value = ?)`, path, value)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func queryExists(db *sql.DB, path string) ([]kernel.ObjectID, error) {
	rows, err := db.Query(`SELECT DISTINCT object_id FROM fields WHERE path = ?`, path)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func queryCompare(db *sql.DB, c reader.SearchClause, numeric bool) ([]kernel.ObjectID, error) {
	op := map[reader.SearchOp]string{
		reader.OpGT: ">", reader.OpGTE: ">=", reader.OpLT: "<", reader.OpLTE: "<=",
	}[c.Op]
	expr := "value"
	if numeric {
		expr = "CAST(value AS REAL)"
	}
	rows, err := db.Query(`SELECT object_id FROM fields WHERE path = ? AND `+expr+` `+op+` ?`, c.Path, c.Value)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func queryMatchPath(db *sql.DB, path, text string) ([]kernel.ObjectID, error) {
	tokens := matchTokens(text)
	if len(tokens) == 0 {
		return nil, nil
	}
	rows, err := db.Query(`SELECT object_id, value FROM fields WHERE path = ?`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []kernel.ObjectID
	for rows.Next() {
		var id, value string
		if err := rows.Scan(&id, &value); err != nil {
			return nil, err
		}
		lower := strings.ToLower(value)
		ok := true
		for _, tok := range tokens {
			if !strings.Contains(lower, tok) {
				ok = false
				break
			}
		}
		if ok {
			ids = append(ids, kernel.ObjectID(id))
		}
	}
	return ids, rows.Err()
}

func matchTokens(text string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(text)) {
		w = ftsUnsafe.ReplaceAllString(w, "")
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

func orderIDs(db *sql.DB, ids []kernel.ObjectID, path string, desc, numeric bool) ([]kernel.ObjectID, error) {
	if len(ids) == 0 {
		return ids, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, 1+len(ids))
	args = append(args, path)
	seen := map[kernel.ObjectID]struct{}{}
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, string(id))
		seen[id] = struct{}{}
	}
	expr := "value"
	if numeric {
		expr = "CAST(value AS REAL)"
	}
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	rows, err := db.Query(`SELECT object_id FROM fields WHERE path = ? AND object_id IN (`+strings.Join(placeholders, ",")+`) ORDER BY `+expr+` `+dir, args...)
	if err != nil {
		return nil, err
	}
	ordered, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	have := map[kernel.ObjectID]struct{}{}
	var out []kernel.ObjectID
	for _, id := range ordered {
		have[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range ids {
		if _, ok := have[id]; !ok {
			out = append(out, id)
		}
	}
	return out, nil
}
