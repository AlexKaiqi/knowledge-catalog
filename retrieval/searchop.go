package retrieval

import (
	"strings"

	"kc/kernel"
	"kc/knowledge/reader"
)

// SearchOp is a query-time use of a field, not an index declaration.
// Declaration is schema field access[] + type (AccessHint). AllowsOp is the implied table.
// Clauses AND together. OR / NOT / grouping wait for a query language.
type SearchOp string

const (
	OpMatch   SearchOp = "MATCH"
	OpEQ      SearchOp = "EQ"
	OpIN      SearchOp = "IN"
	OpNEQ     SearchOp = "NEQ"
	OpExists  SearchOp = "EXISTS"
	OpMissing SearchOp = "MISSING"
	OpPrefix  SearchOp = "PREFIX"
	OpGT      SearchOp = "GT"
	OpGTE     SearchOp = "GTE"
	OpLT      SearchOp = "LT"
	OpLTE     SearchOp = "LTE"
	OpSort    SearchOp = "SORT"
)

type MatchMode string

const (
	MatchAllTerms MatchMode = "AllTerms"
	MatchAnyTerms MatchMode = "AnyTerms"
	MatchPhrase   MatchMode = "Phrase"
)

type SearchClause struct {
	Op     SearchOp  `json:"op"`
	Path   string    `json:"path,omitempty"`
	Field  *FieldRef `json:"field,omitempty"`
	Value  string    `json:"value,omitempty"`
	Values []string  `json:"values,omitempty"`
	Order  string    `json:"order,omitempty"`
	Mode   MatchMode `json:"mode,omitempty"`
}

func (c SearchClause) Locates() bool { return c.Op != OpSort }

type SearchRequest struct {
	Clauses      []SearchClause `json:"clauses"`
	Limit        int            `json:"limit,omitempty"`
	Continuation string         `json:"continuation,omitempty"`
}

func SearchMATCH(text string) SearchClause {
	return SearchClause{Op: OpMatch, Value: text, Mode: MatchAllTerms}
}

func SearchMATCHMode(text string, mode MatchMode) SearchClause {
	return SearchClause{Op: OpMatch, Value: text, Mode: mode}
}

func SearchEQ(path, value string) SearchClause {
	return SearchClause{Op: OpEQ, Path: path, Value: value}
}

func SearchIN(path string, values ...string) SearchClause {
	return SearchClause{Op: OpIN, Path: path, Values: append([]string{}, values...)}
}

func SearchNEQ(path, value string) SearchClause {
	return SearchClause{Op: OpNEQ, Path: path, Value: value}
}

func SearchEXISTS(path string) SearchClause {
	return SearchClause{Op: OpExists, Path: path}
}

func SearchMISSING(path string) SearchClause {
	return SearchClause{Op: OpMissing, Path: path}
}

func SearchPREFIX(path, value string) SearchClause {
	return SearchClause{Op: OpPrefix, Path: path, Value: value}
}

func SearchRange(op SearchOp, path, value string) SearchClause {
	return SearchClause{Op: op, Path: path, Value: value}
}

func SearchSORT(path, order string) SearchClause {
	return SearchClause{Op: OpSort, Path: path, Order: order}
}

func SearchOf(clauses ...SearchClause) SearchRequest {
	return SearchRequest{Clauses: append([]SearchClause{}, clauses...)}
}

func ValidateSearch(req SearchRequest) error {
	located := 0
	sorts := 0
	for i, c := range req.Clauses {
		if err := validateClause(c); err != nil {
			return err
		}
		if c.Locates() {
			located++
		} else {
			sorts++
			if sorts > 1 {
				return kernel.Fail(kernel.ErrUsageInvalid, "search allows at most one SORT")
			}
		}
		req.Clauses[i] = c
	}
	if located == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "search requires MATCH, EQ, IN, NEQ, EXISTS, MISSING, PREFIX, or a comparison")
	}
	return nil
}

func validateClause(c SearchClause) error {
	hasField := c.Path != "" || (c.Field != nil && c.Field.Path != "")
	switch c.Op {
	case OpMatch:
		if strings.TrimSpace(c.Value) == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "MATCH requires a value")
		}
		switch c.Mode {
		case "", MatchAllTerms, MatchAnyTerms, MatchPhrase:
		default:
			return kernel.Fail(kernel.ErrUsageInvalid, "unknown MATCH mode %q", c.Mode)
		}
	case OpEQ, OpNEQ, OpPrefix, OpGT, OpGTE, OpLT, OpLTE:
		if !hasField || c.Value == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "%s requires path and value", c.Op)
		}
	case OpIN:
		if !hasField || len(c.Values) == 0 {
			return kernel.Fail(kernel.ErrUsageInvalid, "IN requires path and values")
		}
	case OpExists, OpMissing:
		if !hasField {
			return kernel.Fail(kernel.ErrUsageInvalid, "EXISTS requires path")
		}
	case OpSort:
		if !hasField {
			return kernel.Fail(kernel.ErrUsageInvalid, "SORT requires path")
		}
		switch strings.ToLower(c.Order) {
		case "", "asc", "desc":
		default:
			return kernel.Fail(kernel.ErrUsageInvalid, "SORT order must be asc or desc")
		}
	default:
		return kernel.Fail(kernel.ErrUsageInvalid, "unknown search operator %s", c.Op)
	}
	return nil
}

// AllowsOp is the implied table: access[] + type ⇒ which query ops may touch this field.
// Schema does not list EQ/IN/MATCH. text → MATCH; filter → EQ/IN/NEQ/EXISTS;
// filter plus comparable type → GT/GTE/LT/LTE; sort → SORT.
func AllowsOp(field AccessField, op SearchOp) bool {
	switch op {
	case OpMatch:
		return field.Has(reader.HintText)
	case OpEQ, OpIN, OpNEQ, OpExists, OpMissing:
		return field.Has(reader.HintFilter)
	case OpPrefix:
		return field.Has(reader.HintFilter) && StringType(field.Type)
	case OpGT, OpGTE, OpLT, OpLTE:
		return field.Has(reader.HintFilter) && RangeType(field.Type)
	case OpSort:
		return field.Has(reader.HintSort)
	default:
		return false
	}
}

// CheckSearch: each clause.Op must be implied by that path's AccessHints. Bare MATCH
// (no path) fans out across text fields. Failure is CAPABILITY_UNSATISFIED, not JSON contains.
func CheckSearch(req SearchRequest, spec AccessSpec) error {
	if err := ValidateSearch(req); err != nil {
		return err
	}
	for _, c := range req.Clauses {
		if err := checkClause(c, spec); err != nil {
			return err
		}
	}
	return nil
}

func checkClause(c SearchClause, spec AccessSpec) error {
	if c.Op == OpMatch && c.Path == "" && c.Field == nil {
		if !spec.HasHint(reader.HintText) {
			return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "MATCH needs text access")
		}
		return nil
	}
	if c.Path == "" && (c.Field == nil || c.Field.Path == "") {
		return kernel.Fail(kernel.ErrUsageInvalid, "%s requires path", c.Op)
	}
	if len(spec.Fields) == 0 {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "structured %s needs AccessHints; path %s is unplanned", c.Op, c.Path)
	}
	ref := FieldRef{Path: c.Path}
	if c.Field != nil {
		ref = *c.Field
	}
	field, err := spec.ResolveField(ref)
	if err != nil {
		return err
	}
	if !AllowsOp(field, c.Op) {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "%s is not implied by access %v type %q on %s", c.Op, field.Access, field.Type, c.Path)
	}
	return nil
}

func ResolveSearch(req SearchRequest, spec AccessSpec) (SearchRequest, error) {
	if err := CheckSearch(req, spec); err != nil {
		return SearchRequest{}, err
	}
	out := SearchRequest{Clauses: append([]SearchClause(nil), req.Clauses...), Limit: req.Limit, Continuation: req.Continuation}
	for i := range out.Clauses {
		resolved, err := ResolveSearchClause(out.Clauses[i], spec)
		if err != nil {
			return SearchRequest{}, err
		}
		out.Clauses[i] = resolved
	}
	return out, nil
}

func ResolveSearchClause(clause SearchClause, spec AccessSpec) (SearchClause, error) {
	if err := validateClause(clause); err != nil {
		return SearchClause{}, err
	}
	if err := checkClause(clause, spec); err != nil {
		return SearchClause{}, err
	}
	if clause.Op == OpMatch && clause.Mode == "" {
		clause.Mode = MatchAllTerms
	}
	if clause.Op == OpMatch && clause.Path == "" && clause.Field == nil {
		return clause, nil
	}
	ref := FieldRef{Path: clause.Path}
	if clause.Field != nil {
		ref = *clause.Field
	}
	field, err := spec.ResolveField(ref)
	if err != nil {
		return SearchClause{}, err
	}
	resolved := field.FieldRef
	clause.Field = &resolved
	clause.Path = resolved.Key()
	if clause.Op != OpMatch && clause.Op != OpExists && clause.Op != OpMissing && clause.Op != OpSort {
		if clause.Op != OpIN {
			clause.Value, err = NormalizeScalarLiteral(field.Type, clause.Value)
			if err != nil {
				return SearchClause{}, err
			}
		}
		for j := range clause.Values {
			clause.Values[j], err = NormalizeScalarLiteral(field.Type, clause.Values[j])
			if err != nil {
				return SearchClause{}, err
			}
		}
	}
	return clause, nil
}

func RangeType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "int", "integer", "long", "number", "float", "double", "date", "datetime", "timestamp":
		return true
	default:
		return false
	}
}

func ComparableType(t string) bool { return RangeType(t) || StringType(t) }

func StringType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "", "string":
		return true
	default:
		return false
	}
}

func TemporalType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "date", "datetime", "timestamp":
		return true
	default:
		return false
	}
}

func NumericType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "int", "integer", "long", "number", "float", "double":
		return true
	default:
		return false
	}
}
