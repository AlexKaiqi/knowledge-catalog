package sqlite

import (
	"database/sql"
	"kc/retrieval"
	"regexp"
	"sort"
	"strings"

	"kc/kernel"
	"kc/knowledge"
)

func searchIDs(db *sql.DB, req retrieval.SearchRequest, spec retrieval.AccessSpec) ([]knowledge.ObjectID, error) {
	var sets [][]knowledge.ObjectID
	var sortClause retrieval.SearchClause
	for _, c := range req.Clauses {
		if c.Op == retrieval.OpSort {
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

func queryFTS(db *sql.DB, text string, mode retrieval.MatchMode) ([]knowledge.ObjectID, error) {
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

func queryFilter(db *sql.DB, path, value string) ([]knowledge.ObjectID, error) {
	rows, err := db.Query(`SELECT object_id FROM fields WHERE path = ? AND value = ?`, path, value)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func scanIDs(rows *sql.Rows) ([]knowledge.ObjectID, error) {
	defer rows.Close()
	var ids []knowledge.ObjectID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, knowledge.ObjectID(id))
	}
	return ids, rows.Err()
}

func intersectIDs(sets [][]knowledge.ObjectID) []knowledge.ObjectID {
	if len(sets) == 0 {
		return nil
	}
	counts := map[knowledge.ObjectID]int{}
	order := []knowledge.ObjectID{}
	for _, id := range sets[0] {
		if counts[id] == 0 {
			order = append(order, id)
		}
		counts[id] = 1
	}
	for i := 1; i < len(sets); i++ {
		next := map[knowledge.ObjectID]struct{}{}
		for _, id := range sets[i] {
			next[id] = struct{}{}
		}
		var kept []knowledge.ObjectID
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

func ftsMatch(text string, mode retrieval.MatchMode) string {
	var parts []string
	for _, w := range strings.Fields(strings.ToLower(text)) {
		w = ftsUnsafe.ReplaceAllString(w, "")
		if w == "" {
			continue
		}
		parts = append(parts, `"`+w+`"`)
	}
	switch mode {
	case retrieval.MatchAnyTerms:
		return strings.Join(parts, " OR ")
	case retrieval.MatchPhrase:
		tokens := matchTokens(text)
		if len(tokens) == 0 {
			return ""
		}
		return `"` + strings.Join(tokens, " ") + `"`
	default:
		return strings.Join(parts, " AND ")
	}
}
