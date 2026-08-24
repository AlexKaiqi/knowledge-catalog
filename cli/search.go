package cli

import (
	"fmt"
	"strings"

	"kc/reader"
)

func searchRequestFromFlags(flags map[string]FlagValue) (reader.SearchRequest, error) {
	var clauses []reader.SearchClause
	if q := FlagString(flags, "query"); q != "" {
		clauses = append(clauses, reader.SearchMATCH(q))
	}
	for _, item := range FlagStrings(flags, "eq") {
		path, value, err := splitSearchPair(item)
		if err != nil {
			return reader.SearchRequest{}, fmt.Errorf("--eq must be path=value, got %s", item)
		}
		clauses = append(clauses, reader.SearchEQ(path, value))
	}
	for _, item := range FlagStrings(flags, "neq") {
		path, value, err := splitSearchPair(item)
		if err != nil {
			return reader.SearchRequest{}, fmt.Errorf("--neq must be path=value, got %s", item)
		}
		clauses = append(clauses, reader.SearchNEQ(path, value))
	}
	for _, item := range FlagStrings(flags, "in") {
		path, value, err := splitSearchPair(item)
		if err != nil {
			return reader.SearchRequest{}, fmt.Errorf("--in must be path=v1,v2, got %s", item)
		}
		clauses = append(clauses, reader.SearchIN(path, splitCSV(value)...))
	}
	for _, path := range FlagStrings(flags, "exists") {
		if path == "" {
			return reader.SearchRequest{}, fmt.Errorf("--exists requires a path")
		}
		clauses = append(clauses, reader.SearchEXISTS(path))
	}
	for _, op := range []reader.SearchOp{reader.OpGT, reader.OpGTE, reader.OpLT, reader.OpLTE} {
		flag := strings.ToLower(string(op))
		for _, item := range FlagStrings(flags, flag) {
			path, value, err := splitSearchPair(item)
			if err != nil {
				return reader.SearchRequest{}, fmt.Errorf("--%s must be path=value, got %s", flag, item)
			}
			clauses = append(clauses, reader.SearchRange(op, path, value))
		}
	}
	if raw := FlagString(flags, "sort"); raw != "" {
		path, order := raw, "asc"
		if i := strings.LastIndexByte(raw, ':'); i >= 0 {
			path, order = raw[:i], raw[i+1:]
		}
		clauses = append(clauses, reader.SearchSORT(path, order))
	}
	for _, item := range FlagStrings(flags, "match") {
		path, value, err := splitSearchPair(item)
		if err != nil {
			return reader.SearchRequest{}, fmt.Errorf("--match must be path=text, got %s", item)
		}
		clauses = append(clauses, reader.SearchClause{Op: reader.OpMatch, Path: path, Value: value})
	}
	return reader.SearchOf(clauses...), nil
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
