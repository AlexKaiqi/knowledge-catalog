package local

import (
	"database/sql"
	"regexp"
	"sort"
	"strings"

	"kc/kernel"
	"kc/reader"
)

func searchIDs(db *sql.DB, req reader.SearchRequest, spec reader.AccessSpec) ([]kernel.ObjectID, error) {
	var sets [][]kernel.ObjectID
	var sortClause reader.SearchClause
	for _, c := range req.Clauses {
		if c.Op == reader.OpSort {
			sortClause = c
			continue
		}
		ids, err := clauseIDs(db, c, spec)
		if err != nil {
			return nil, err
		}
		sets = append(sets, ids)
	}
	if len(sets) == 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "search requires a locating clause")
	}
	ids := intersectIDs(sets)
	if sortClause.Path == "" {
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		return ids, nil
	}
	field, err := spec.ResolveField(*sortClause.Field)
	if err != nil {
		return nil, err
	}
	return orderIDs(db, ids, sortClause.Path, strings.EqualFold(sortClause.Order, "desc"), field.Type)
}

func queryFTS(db *sql.DB, text string, mode reader.MatchMode) ([]kernel.ObjectID, error) {
	match := ftsMatch(text, mode)
	if match == "" {
		return nil, nil
	}
	rows, err := db.Query(`SELECT object_id FROM fts WHERE fts MATCH ?`, match)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func queryFilter(db *sql.DB, path, value string) ([]kernel.ObjectID, error) {
	rows, err := db.Query(`SELECT object_id FROM fields WHERE path = ? AND value = ?`, path, value)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func scanIDs(rows *sql.Rows) ([]kernel.ObjectID, error) {
	defer rows.Close()
	var ids []kernel.ObjectID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, kernel.ObjectID(id))
	}
	return ids, rows.Err()
}

func intersectIDs(sets [][]kernel.ObjectID) []kernel.ObjectID {
	if len(sets) == 0 {
		return nil
	}
	counts := map[kernel.ObjectID]int{}
	order := []kernel.ObjectID{}
	for _, id := range sets[0] {
		if counts[id] == 0 {
			order = append(order, id)
		}
		counts[id] = 1
	}
	for i := 1; i < len(sets); i++ {
		next := map[kernel.ObjectID]struct{}{}
		for _, id := range sets[i] {
			next[id] = struct{}{}
		}
		var kept []kernel.ObjectID
		for _, id := range order {
			if _, ok := next[id]; ok && counts[id] == i {
				counts[id] = i + 1
				kept = append(kept, id)
			}
		}
		order = kept
	}
	return order
}

var ftsUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_\p{L}]+`)

func ftsMatch(text string, mode reader.MatchMode) string {
	var parts []string
	for _, w := range strings.Fields(strings.ToLower(text)) {
		w = ftsUnsafe.ReplaceAllString(w, "")
		if w == "" {
			continue
		}
		parts = append(parts, `"`+w+`"`)
	}
	switch mode {
	case reader.MatchAnyTerms:
		return strings.Join(parts, " OR ")
	case reader.MatchPhrase:
		tokens := matchTokens(text)
		if len(tokens) == 0 {
			return ""
		}
		return `"` + strings.Join(tokens, " ") + `"`
	default:
		return strings.Join(parts, " AND ")
	}
}
