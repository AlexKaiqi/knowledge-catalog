package index

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"kc/kernel"
	"kc/reader"
	"kc/repository"
)

var residualTokenUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_\p{L}]+`)

func lookupPath(value any, path string) (any, bool) {
	if path == "" {
		return value, value != nil
	}
	cur := value
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			continue
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func fieldValue(assembled any, aspect, path string) (any, bool) {
	if aspect != "" {
		if obj, ok := assembled.(map[string]any); ok {
			if inner, ok := obj[aspect]; ok {
				if v, ok := lookupPath(inner, path); ok {
					return v, true
				}
			}
		}
	}
	return lookupPath(assembled, path)
}

func scalarString(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(b)
}

// boundSpec is the AccessSpec that applies to one object: recipe objects contribute
// nothing; schema_ref selects which declared schemas; otherwise the full spec.
func boundSpec(repo repository.Repository, value repository.KnowledgeValue, spec reader.AccessSpec) reader.AccessSpec {
	if kernel.IsSchemaObject(value.Address.ObjectID) {
		return reader.AccessSpec{AccessDigest: spec.AccessDigest}
	}
	return spec.Bind(objectSchemaRefs(repo, value))
}

func objectSchemaRefs(repo repository.Repository, value repository.KnowledgeValue) []string {
	if repo == nil {
		return nil
	}
	if len(value.Units) == 0 {
		res, err := repo.Resolve(value.Address.ObjectID, value.Commit)
		if err != nil || res.SchemaRef == "" {
			return nil
		}
		return []string{res.SchemaRef}
	}
	var refs []string
	seen := map[string]struct{}{}
	for _, addr := range value.Units {
		res, err := repo.ResolveAddress(addr, value.Commit)
		if err != nil || res.SchemaRef == "" {
			continue
		}
		if _, ok := seen[res.SchemaRef]; ok {
			continue
		}
		seen[res.SchemaRef] = struct{}{}
		refs = append(refs, res.SchemaRef)
	}
	return refs
}

func documentText(value repository.KnowledgeValue, spec reader.AccessSpec) string {
	assembled := value.Value
	var parts []string
	for _, field := range spec.Fields {
		if !field.Has(reader.HintText) {
			continue
		}
		if v, ok := fieldValue(assembled, field.Aspect, field.Path); ok {
			values := []any{v}
			if list, ok := v.([]any); ok {
				values = list
			}
			for _, item := range values {
				if s := scalarString(item); s != "" {
					parts = append(parts, s)
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

func indexedFields(value repository.KnowledgeValue, spec reader.AccessSpec) [][2]string {
	assembled := value.Value
	var out [][2]string
	seen := map[string]struct{}{}
	for _, field := range spec.Fields {
		if !field.Has(reader.HintFilter) && !field.Has(reader.HintSort) && !field.Has(reader.HintText) {
			continue
		}
		v, ok := fieldValue(assembled, field.Aspect, field.Path)
		if !ok {
			continue
		}
		key := field.FieldRef.Key()
		values := []any{v}
		if list, ok := v.([]any); ok {
			values = list
		}
		for _, item := range values {
			s, valid := reader.NormalizeScalarValue(field.Type, item)
			if !valid {
				continue
			}
			identity := key + "\x00" + s
			if _, dup := seen[identity]; dup {
				continue
			}
			seen[identity] = struct{}{}
			out = append(out, [2]string{key, s})
		}
	}
	return out
}

// matchesResidual evaluates the logical predicate against the hydrated
// Canonical value. It is used only when a provider promises a Superset.
func matchesResidual(repo repository.Repository, value repository.KnowledgeValue, req reader.SearchRequest, spec reader.AccessSpec) (bool, error) {
	bound := boundSpec(repo, value, spec)
	for _, clause := range req.Clauses {
		if clause.Op == reader.OpSort {
			continue
		}
		if clause.Op == reader.OpMatch && clause.Field == nil && clause.Path == "" {
			if !residualMatch(documentText(value, bound), clause.Value, clause.Mode) {
				return false, nil
			}
			continue
		}
		field, err := bound.ResolveField(*clause.Field)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
				return false, nil
			}
			return false, err
		}
		raw, exists := fieldValue(value.Value, field.Aspect, field.Path)
		values := residualValues(field.Type, raw, exists)
		switch clause.Op {
		case reader.OpMatch:
			matched := false
			for _, item := range rawValues(raw, exists) {
				if residualMatch(scalarString(item), clause.Value, clause.Mode) {
					matched = true
					break
				}
			}
			if !matched {
				return false, nil
			}
		case reader.OpExists:
			if len(values) == 0 {
				return false, nil
			}
		case reader.OpMissing:
			if len(values) != 0 {
				return false, nil
			}
		case reader.OpEQ, reader.OpIN, reader.OpNEQ, reader.OpPrefix, reader.OpGT, reader.OpGTE, reader.OpLT, reader.OpLTE:
			if !residualScalarClause(clause, field.Type, values) {
				return false, nil
			}
		}
	}
	return true, nil
}

func rawValues(raw any, exists bool) []any {
	if !exists || raw == nil {
		return nil
	}
	if list, ok := raw.([]any); ok {
		return list
	}
	return []any{raw}
}

func residualValues(fieldType string, raw any, exists bool) []string {
	var out []string
	for _, item := range rawValues(raw, exists) {
		if normalized, ok := reader.NormalizeScalarValue(fieldType, item); ok {
			out = append(out, normalized)
		}
	}
	return out
}

func residualScalarClause(clause reader.SearchClause, fieldType string, values []string) bool {
	switch clause.Op {
	case reader.OpEQ:
		return containsScalar(values, clause.Value)
	case reader.OpIN:
		for _, target := range clause.Values {
			if containsScalar(values, target) {
				return true
			}
		}
		return false
	case reader.OpNEQ:
		return len(values) > 0 && !containsScalar(values, clause.Value)
	case reader.OpPrefix:
		for _, value := range values {
			if strings.HasPrefix(value, clause.Value) {
				return true
			}
		}
		return false
	case reader.OpGT, reader.OpGTE, reader.OpLT, reader.OpLTE:
		for _, value := range values {
			if scalarCompare(value, clause.Value, fieldType, clause.Op) {
				return true
			}
		}
	}
	return false
}

func containsScalar(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func scalarCompare(left, right, fieldType string, op reader.SearchOp) bool {
	cmp := strings.Compare(left, right)
	if reader.NumericType(fieldType) {
		l, lerr := strconv.ParseFloat(left, 64)
		r, rerr := strconv.ParseFloat(right, 64)
		if lerr != nil || rerr != nil {
			return false
		}
		switch {
		case l < r:
			cmp = -1
		case l > r:
			cmp = 1
		default:
			cmp = 0
		}
	}
	if reader.TemporalType(fieldType) {
		layout := time.RFC3339
		if strings.EqualFold(strings.TrimSpace(fieldType), "date") {
			layout = "2006-01-02"
		}
		l, lerr := time.Parse(layout, left)
		r, rerr := time.Parse(layout, right)
		if lerr != nil || rerr != nil {
			return false
		}
		switch {
		case l.Before(r):
			cmp = -1
		case l.After(r):
			cmp = 1
		default:
			cmp = 0
		}
	}
	switch op {
	case reader.OpGT:
		return cmp > 0
	case reader.OpGTE:
		return cmp >= 0
	case reader.OpLT:
		return cmp < 0
	case reader.OpLTE:
		return cmp <= 0
	default:
		return false
	}
}

func residualMatch(text, query string, mode reader.MatchMode) bool {
	text = strings.ToLower(text)
	var terms []string
	for _, term := range strings.Fields(strings.ToLower(query)) {
		term = residualTokenUnsafe.ReplaceAllString(term, "")
		if term != "" {
			terms = append(terms, term)
		}
	}
	if len(terms) == 0 {
		return false
	}
	if mode == reader.MatchPhrase {
		return strings.Contains(text, strings.Join(terms, " "))
	}
	for _, term := range terms {
		found := strings.Contains(text, term)
		if mode == reader.MatchAnyTerms && found {
			return true
		}
		if mode != reader.MatchAnyTerms && !found {
			return false
		}
	}
	return mode != reader.MatchAnyTerms
}
