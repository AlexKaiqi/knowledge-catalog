package invocation

// Request-shape derivation: pure flag -> coordinate parsers shared by the
// verbs, the typed service, and the evidence pipeline (CAP-ORG-01 B5b.3).

import (
	"fmt"
	"strconv"
	"strings"

	"kc/invoke"
	"kc/kernel"
	knowledge "kc/knowledge"
	"kc/retrieval"
)

// targetRef is the write target, defaulting to the protocol branch.
// limitFrom reads --limit. A missing flag falls back; a non-numeric or negative
// value is a usage error. Verbs used to disagree here — one silently treated
// garbage as zero — which made `kc log --limit abc` quietly return everything.
func LimitFrom(flags map[string]invoke.FlagValue, fallback int) (int, error) {
	raw := invoke.FlagString(flags, "limit")
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("--limit must be a non-negative number")
	}
	return n, nil
}

// PageLimit treats an omitted flag and an explicit 0 as the default page.
// Callers cannot use limit=0 as an unbounded export.
func PageLimit(flags map[string]invoke.FlagValue, fallback, max int) (int, error) {
	n, err := LimitFrom(flags, fallback)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		n = fallback
	}
	if max > 0 && n > max {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "page limit cannot exceed %d", max)
	}
	return n, nil
}

// UsesAddress reports whether the caller named an Aspect or Member unit.
// --member without --aspect is still an Address attempt and must not fall
// through to object-level RESOLVE/READ.
func UsesAddress(flags map[string]invoke.FlagValue) bool {
	return invoke.FlagString(flags, "aspect") != "" || invoke.FlagString(flags, "member") != ""
}

// addressFrom builds the write/read unit key: object_id plus optional aspect and
// member. --member without --aspect is not an address.
func AddressFrom(flags map[string]invoke.FlagValue) (knowledge.Address, error) {
	objectID, err := invoke.RequireFlag(flags, "object")
	if err != nil {
		return knowledge.Address{}, err
	}
	aspect := invoke.FlagString(flags, "aspect")
	member := invoke.FlagString(flags, "member")
	if member != "" {
		if aspect == "" {
			return knowledge.Address{}, kernel.Fail(kernel.ErrUsageInvalid, "member address requires --aspect and --member")
		}
		return knowledge.Address{Kind: knowledge.KindMember, ObjectID: knowledge.ObjectID(objectID), AspectName: aspect, MemberKey: member}, nil
	}
	if aspect != "" {
		return knowledge.Address{Kind: knowledge.KindAspect, ObjectID: knowledge.ObjectID(objectID), AspectName: aspect}, nil
	}
	kind := knowledge.AddressKind(invoke.FlagString(flags, "kind"))
	if kind == "" {
		kind = knowledge.KindEntity
	}
	return knowledge.Address{Kind: kind, ObjectID: knowledge.ObjectID(objectID)}, nil
}

func SearchRequestFromFlags(flags map[string]invoke.FlagValue) (retrieval.SearchRequest, error) {
	if request, ok := flags["_search-request"].(retrieval.SearchRequest); ok {
		return request, retrieval.ValidateSearch(request)
	}
	var clauses []retrieval.SearchClause
	limit, err := LimitFrom(flags, 0)
	if err != nil {
		return retrieval.SearchRequest{}, err
	}
	mode, err := MatchModeFrom(flags)
	if err != nil {
		return retrieval.SearchRequest{}, err
	}
	if q := invoke.FlagString(flags, "query"); q != "" {
		clauses = append(clauses, retrieval.SearchMATCHMode(q, mode))
	}
	for _, item := range invoke.FlagStrings(flags, "eq") {
		path, value, err := SplitSearchPair(item)
		if err != nil {
			return retrieval.SearchRequest{}, fmt.Errorf("--eq must be path=value, got %s", item)
		}
		clauses = append(clauses, SearchFieldClause(retrieval.SearchEQ(path, value), path))
	}
	for _, item := range invoke.FlagStrings(flags, "neq") {
		path, value, err := SplitSearchPair(item)
		if err != nil {
			return retrieval.SearchRequest{}, fmt.Errorf("--neq must be path=value, got %s", item)
		}
		clauses = append(clauses, SearchFieldClause(retrieval.SearchNEQ(path, value), path))
	}
	for _, item := range invoke.FlagStrings(flags, "in") {
		path, value, err := SplitSearchPair(item)
		if err != nil {
			return retrieval.SearchRequest{}, fmt.Errorf("--in must be path=v1,v2, got %s", item)
		}
		clauses = append(clauses, SearchFieldClause(retrieval.SearchIN(path, SplitCSV(value)...), path))
	}
	for _, path := range invoke.FlagStrings(flags, "exists") {
		if path == "" {
			return retrieval.SearchRequest{}, fmt.Errorf("--exists requires a path")
		}
		clauses = append(clauses, SearchFieldClause(retrieval.SearchEXISTS(path), path))
	}
	for _, path := range invoke.FlagStrings(flags, "missing") {
		if path == "" {
			return retrieval.SearchRequest{}, fmt.Errorf("--missing requires a path")
		}
		clauses = append(clauses, SearchFieldClause(retrieval.SearchMISSING(path), path))
	}
	for _, item := range invoke.FlagStrings(flags, "prefix") {
		path, value, err := SplitSearchPair(item)
		if err != nil {
			return retrieval.SearchRequest{}, fmt.Errorf("--prefix must be path=value, got %s", item)
		}
		clauses = append(clauses, SearchFieldClause(retrieval.SearchPREFIX(path, value), path))
	}
	for _, item := range invoke.FlagStrings(flags, "contains") {
		path, value, err := SplitSearchPair(item)
		if err != nil {
			return retrieval.SearchRequest{}, fmt.Errorf("--contains must be path=value, got %s", item)
		}
		clauses = append(clauses, SearchFieldClause(retrieval.SearchCONTAINS(path, value), path))
	}
	for _, op := range []retrieval.SearchOp{retrieval.OpGT, retrieval.OpGTE, retrieval.OpLT, retrieval.OpLTE} {
		flag := strings.ToLower(string(op))
		for _, item := range invoke.FlagStrings(flags, flag) {
			path, value, err := SplitSearchPair(item)
			if err != nil {
				return retrieval.SearchRequest{}, fmt.Errorf("--%s must be path=value, got %s", flag, item)
			}
			clauses = append(clauses, SearchFieldClause(retrieval.SearchRange(op, path, value), path))
		}
	}
	if raw := invoke.FlagString(flags, "sort"); raw != "" {
		path, order := raw, "asc"
		if i := strings.LastIndexByte(raw, ':'); i >= 0 {
			path, order = raw[:i], raw[i+1:]
		}
		clauses = append(clauses, SearchFieldClause(retrieval.SearchSORT(path, order), path))
	}
	for _, item := range invoke.FlagStrings(flags, "match") {
		path, value, err := SplitSearchPair(item)
		if err != nil {
			return retrieval.SearchRequest{}, fmt.Errorf("--match must be path=text, got %s", item)
		}
		clauses = append(clauses, SearchFieldClause(retrieval.SearchClause{Op: retrieval.OpMatch, Path: path, Value: value, Mode: mode}, path))
	}
	req := retrieval.SearchOf(clauses...)
	req.Limit = limit
	req.Continuation = invoke.FlagString(flags, "continuation")
	recall := strings.ToLower(strings.TrimSpace(invoke.FlagString(flags, "recall")))
	if recall != "" {
		req.Recall = retrieval.RecallStrategy(recall)
	}
	if err := retrieval.ValidateRecall(req); err != nil {
		return retrieval.SearchRequest{}, err
	}
	return req, nil
}

func MatchModeFrom(flags map[string]invoke.FlagValue) (retrieval.MatchMode, error) {
	raw := strings.TrimSpace(invoke.FlagString(flags, "match-mode"))
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
func SearchFieldClause(clause retrieval.SearchClause, raw string) retrieval.SearchClause {
	parts := strings.Split(raw, "::")
	if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[2]) == "" {
		return clause
	}
	ref := retrieval.FieldRef{Schema: knowledge.ObjectID(strings.TrimSpace(parts[0])), Aspect: strings.TrimSpace(parts[1]), Path: strings.TrimSpace(parts[2])}
	clause.Field = &ref
	clause.Path = ""
	return clause
}

func SplitSearchPair(item string) (string, string, error) {
	eq := strings.IndexByte(item, '=')
	if eq < 0 || item[:eq] == "" {
		return "", "", fmt.Errorf("not path=value")
	}
	return item[:eq], item[eq+1:], nil
}

func SplitCSV(raw string) []string {
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

func TraverseHops(flags map[string]invoke.FlagValue) (int, error) {
	raw := invoke.FlagString(flags, "max-hops")
	if raw == "" {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "traverse requires --max-hops")
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "--max-hops must be a number")
	}
	return n, nil
}
