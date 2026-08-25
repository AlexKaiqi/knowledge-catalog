package sqlite

import (
	"database/sql"
	"sort"
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/reader"
)

func clauseIDs(db *sql.DB, c reader.SearchClause, spec reader.AccessSpec) ([]knowledge.ObjectID, error) {
	switch c.Op {
	case reader.OpMatch:
		if c.Path == "" {
			if containsNonASCII(c.Value) {
				return queryDeclaredTextFields(db, c.Value, c.Mode, spec)
			}
			return queryFTS(db, c.Value, c.Mode)
		}
		return queryMatchPath(db, c.Path, c.Value, c.Mode)
	case reader.OpEQ:
		return queryFilter(db, c.Path, c.Value)
	case reader.OpIN:
		return queryIN(db, c.Path, c.Values)
	case reader.OpNEQ:
		return queryNEQ(db, c.Path, c.Value)
	case reader.OpExists:
		return queryExists(db, c.Path)
	case reader.OpMissing:
		return queryMissing(db, c.Path)
	case reader.OpPrefix:
		return queryPrefix(db, c.Path, c.Value)
	case reader.OpGT, reader.OpGTE, reader.OpLT, reader.OpLTE:
		field, err := spec.ResolveField(*c.Field)
		if err != nil {
			return nil, err
		}
		return queryCompare(db, c, field.Type)
	default:
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown search operator %s", c.Op)
	}
}

func containsNonASCII(value string) bool {
	for _, r := range value {
		if r > 127 {
			return true
		}
	}
	return false
}

// queryDeclaredTextFields is the exact local-reference fallback for scripts
// that SQLite FTS unicode61 does not segment into useful search tokens (for
// example, contiguous CJK text). It scans only fields explicitly declared
// with text access; it never falls back to whole-object JSON contains.
func queryDeclaredTextFields(db *sql.DB, text string, mode reader.MatchMode, spec reader.AccessSpec) ([]knowledge.ObjectID, error) {
	tokens := matchTokens(text)
	if len(tokens) == 0 {
		return nil, nil
	}
	seenPaths := map[string]struct{}{}
	paths := []string{}
	for _, field := range spec.Fields {
		if !field.Has(reader.HintText) {
			continue
		}
		path := field.FieldRef.Key()
		if _, exists := seenPaths[path]; exists {
			continue
		}
		seenPaths[path] = struct{}{}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(paths))
	args := make([]any, len(paths))
	for i, path := range paths {
		placeholders[i] = "?"
		args[i] = path
	}
	rows, err := db.Query(`SELECT object_id, value FROM fields WHERE path IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[knowledge.ObjectID][]string{}
	for rows.Next() {
		var id knowledge.ObjectID
		var value string
		if err := rows.Scan(&id, &value); err != nil {
			return nil, err
		}
		values[id] = append(values[id], value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ids := []knowledge.ObjectID{}
	for id, parts := range values {
		if matchText(strings.ToLower(strings.Join(parts, " ")), tokens, mode) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func queryIN(db *sql.DB, path string, values []string) ([]knowledge.ObjectID, error) {
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

func queryNEQ(db *sql.DB, path, value string) ([]knowledge.ObjectID, error) {
	rows, err := db.Query(`
		SELECT object_id FROM objects
		WHERE object_id IN (SELECT object_id FROM fields WHERE path = ?)
		AND object_id NOT IN (SELECT object_id FROM fields WHERE path = ? AND value = ?)`, path, path, value)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func queryMissing(db *sql.DB, path string) ([]knowledge.ObjectID, error) {
	rows, err := db.Query(`SELECT object_id FROM objects WHERE object_id NOT IN (SELECT object_id FROM fields WHERE path = ?)`, path)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func queryPrefix(db *sql.DB, path, value string) ([]knowledge.ObjectID, error) {
	rows, err := db.Query(`SELECT object_id FROM fields WHERE path = ? AND substr(value, 1, length(?)) = ?`, path, value, value)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func queryExists(db *sql.DB, path string) ([]knowledge.ObjectID, error) {
	rows, err := db.Query(`SELECT DISTINCT object_id FROM fields WHERE path = ?`, path)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func queryCompare(db *sql.DB, c reader.SearchClause, fieldType string) ([]knowledge.ObjectID, error) {
	op := map[reader.SearchOp]string{
		reader.OpGT: ">", reader.OpGTE: ">=", reader.OpLT: "<", reader.OpLTE: "<=",
	}[c.Op]
	expr := "value"
	right := "?"
	if reader.NumericType(fieldType) {
		expr = "CAST(value AS REAL)"
		right = "CAST(? AS REAL)"
	} else if reader.TemporalType(fieldType) {
		expr = "julianday(value)"
		right = "julianday(?)"
	}
	rows, err := db.Query(`SELECT object_id FROM fields WHERE path = ? AND `+expr+` `+op+` `+right, c.Path, c.Value)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func queryMatchPath(db *sql.DB, path, text string, mode reader.MatchMode) ([]knowledge.ObjectID, error) {
	tokens := matchTokens(text)
	if len(tokens) == 0 {
		return nil, nil
	}
	rows, err := db.Query(`SELECT object_id, value FROM fields WHERE path = ?`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []knowledge.ObjectID
	for rows.Next() {
		var id, value string
		if err := rows.Scan(&id, &value); err != nil {
			return nil, err
		}
		lower := strings.ToLower(value)
		ok := matchText(lower, tokens, mode)
		if ok {
			ids = append(ids, knowledge.ObjectID(id))
		}
	}
	return ids, rows.Err()
}

func matchText(lower string, tokens []string, mode reader.MatchMode) bool {
	if mode == reader.MatchPhrase {
		return strings.Contains(lower, strings.Join(tokens, " "))
	}
	for _, tok := range tokens {
		found := strings.Contains(lower, tok)
		if mode == reader.MatchAnyTerms && found {
			return true
		}
		if mode != reader.MatchAnyTerms && !found {
			return false
		}
	}
	return mode != reader.MatchAnyTerms
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

func orderIDs(db *sql.DB, ids []knowledge.ObjectID, path string, desc bool, fieldType string) ([]knowledge.ObjectID, error) {
	if len(ids) == 0 {
		return ids, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, 1+len(ids))
	args = append(args, path)
	seen := map[knowledge.ObjectID]struct{}{}
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, string(id))
		seen[id] = struct{}{}
	}
	expr := "value"
	if reader.NumericType(fieldType) {
		expr = "CAST(value AS REAL)"
	} else if reader.TemporalType(fieldType) {
		expr = "julianday(value)"
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
	have := map[knowledge.ObjectID]struct{}{}
	var out []knowledge.ObjectID
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
