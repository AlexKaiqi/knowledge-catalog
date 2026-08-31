package cli

import (
	"fmt"
	"kc/retrieval"
	"strings"

	"kc/knowledge"
)

func searchRequestFromFlags(flags map[string]FlagValue) (retrieval.SearchRequest, error) {
	if request, ok := flags["_search-request"].(retrieval.SearchRequest); ok {
		return request, retrieval.ValidateSearch(request)
	}
	var clauses []retrieval.SearchClause
	limit, err := limitFrom(flags, 0)
	if err != nil {
		return retrieval.SearchRequest{}, err
	}
	mode, err := matchModeFrom(flags)
	if err != nil {
		return retrieval.SearchRequest{}, err
	}
	if q := FlagString(flags, "query"); q != "" {
		clauses = append(clauses, retrieval.SearchMATCHMode(q, mode))
	}
	for _, item := range FlagStrings(flags, "eq") {
		path, value, err := splitSearchPair(item)
		if err != nil {
			return retrieval.SearchRequest{}, fmt.Errorf("--eq must be path=value, got %s", item)
		}
		clauses = append(clauses, searchFieldClause(retrieval.SearchEQ(path, value), path))
	}
	for _, item := range FlagStrings(flags, "neq") {
		path, value, err := splitSearchPair(item)
		if err != nil {
			return retrieval.SearchRequest{}, fmt.Errorf("--neq must be path=value, got %s", item)
		}
		clauses = append(clauses, searchFieldClause(retrieval.SearchNEQ(path, value), path))
	}
	for _, item := range FlagStrings(flags, "in") {
		path, value, err := splitSearchPair(item)
		if err != nil {
			return retrieval.SearchRequest{}, fmt.Errorf("--in must be path=v1,v2, got %s", item)
		}
		clauses = append(clauses, searchFieldClause(retrieval.SearchIN(path, splitCSV(value)...), path))
	}
	for _, path := range FlagStrings(flags, "exists") {
		if path == "" {
			return retrieval.SearchRequest{}, fmt.Errorf("--exists requires a path")
		}
		clauses = append(clauses, searchFieldClause(retrieval.SearchEXISTS(path), path))
	}
	for _, path := range FlagStrings(flags, "missing") {
		if path == "" {
			return retrieval.SearchRequest{}, fmt.Errorf("--missing requires a path")
		}
		clauses = append(clauses, searchFieldClause(retrieval.SearchMISSING(path), path))
	}
	for _, item := range FlagStrings(flags, "prefix") {
		path, value, err := splitSearchPair(item)
		if err != nil {
			return retrieval.SearchRequest{}, fmt.Errorf("--prefix must be path=value, got %s", item)
		}
		clauses = append(clauses, searchFieldClause(retrieval.SearchPREFIX(path, value), path))
	}
	for _, op := range []retrieval.SearchOp{retrieval.OpGT, retrieval.OpGTE, retrieval.OpLT, retrieval.OpLTE} {
		flag := strings.ToLower(string(op))
		for _, item := range FlagStrings(flags, flag) {
			path, value, err := splitSearchPair(item)
			if err != nil {
				return retrieval.SearchRequest{}, fmt.Errorf("--%s must be path=value, got %s", flag, item)
			}
			clauses = append(clauses, searchFieldClause(retrieval.SearchRange(op, path, value), path))
		}
	}
	if raw := FlagString(flags, "sort"); raw != "" {
		path, order := raw, "asc"
		if i := strings.LastIndexByte(raw, ':'); i >= 0 {
			path, order = raw[:i], raw[i+1:]
		}
		clauses = append(clauses, searchFieldClause(retrieval.SearchSORT(path, order), path))
	}
	for _, item := range FlagStrings(flags, "match") {
		path, value, err := splitSearchPair(item)
		if err != nil {
			return retrieval.SearchRequest{}, fmt.Errorf("--match must be path=text, got %s", item)
		}
		clauses = append(clauses, searchFieldClause(retrieval.SearchClause{Op: retrieval.OpMatch, Path: path, Value: value, Mode: mode}, path))
	}
	req := retrieval.SearchOf(clauses...)
	req.Limit = limit
	req.Continuation = FlagString(flags, "continuation")
	return req, nil
}

func matchModeFrom(flags map[string]FlagValue) (retrieval.MatchMode, error) {
	raw := strings.TrimSpace(FlagString(flags, "match-mode"))
	switch strings.ToLower(raw) {
	case "", "allterms", "all-terms":
		return retrieval.MatchAllTerms, nil
	case "anyterms", "any-terms":
		return retrieval.MatchAnyTerms, nil
	case "phrase":
		return retrieval.MatchPhrase, nil
	default:
		return "", fmt.Errorf("--match-mode must be AllTerms, AnyTerms, or Phrase")
	}
}

// searchFieldClause accepts either a bare path or the explicit
// schema::aspect::path form. Bare paths remain convenient but are rejected by
// AccessSpec when ambiguous.
func searchFieldClause(clause retrieval.SearchClause, raw string) retrieval.SearchClause {
	parts := strings.Split(raw, "::")
	if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[2]) == "" {
		return clause
	}
	ref := retrieval.FieldRef{Schema: knowledge.ObjectID(strings.TrimSpace(parts[0])), Aspect: strings.TrimSpace(parts[1]), Path: strings.TrimSpace(parts[2])}
	clause.Field = &ref
	clause.Path = ""
	return clause
}

func splitSearchPair(item string) (string, string, error) {
	eq := strings.IndexByte(item, '=')
	if eq < 0 || item[:eq] == "" {
		return "", "", fmt.Errorf("not path=value")
	}
	return item[:eq], item[eq+1:], nil
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
