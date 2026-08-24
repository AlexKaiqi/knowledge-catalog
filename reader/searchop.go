package reader

import (
	"strings"

	"kc/kernel"
)

// SearchOp is a query-time use of a field, not an index declaration.
// Declaration is schema field access[] + type (AccessHint). AllowsOp is the implied table.
// Clauses AND together. OR / NOT / grouping wait for a query language.
type SearchOp string

const (
	OpMatch  SearchOp = "MATCH"
	OpEQ     SearchOp = "EQ"
	OpIN     SearchOp = "IN"
	OpNEQ    SearchOp = "NEQ"
	OpExists SearchOp = "EXISTS"
	OpGT     SearchOp = "GT"
	OpGTE    SearchOp = "GTE"
	OpLT     SearchOp = "LT"
	OpLTE    SearchOp = "LTE"
	OpSort   SearchOp = "SORT"
)

type SearchClause struct {
	Op     SearchOp `json:"op"`
	Path   string   `json:"path,omitempty"`
	Value  string   `json:"value,omitempty"`
	Values []string `json:"values,omitempty"`
	Order  string   `json:"order,omitempty"`
}

func (c SearchClause) Locates() bool { return c.Op != OpSort }

type SearchRequest struct {
	Clauses []SearchClause `json:"clauses"`
}

func SearchMATCH(text string) SearchClause {
	return SearchClause{Op: OpMatch, Value: text}
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
		return kernel.Fail(kernel.ErrUsageInvalid, "search requires MATCH, EQ, IN, NEQ, EXISTS, or a comparison")
	}
	return nil
}

func validateClause(c SearchClause) error {
	switch c.Op {
	case OpMatch:
		if strings.TrimSpace(c.Value) == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "MATCH requires a value")
		}
	case OpEQ, OpNEQ, OpGT, OpGTE, OpLT, OpLTE:
		if c.Path == "" || c.Value == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "%s requires path and value", c.Op)
		}
	case OpIN:
		if c.Path == "" || len(c.Values) == 0 {
			return kernel.Fail(kernel.ErrUsageInvalid, "IN requires path and values")
		}
	case OpExists:
		if c.Path == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "EXISTS requires path")
		}
	case OpSort:
		if c.Path == "" {
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
// Schema does not list EQ/IN/MATCH. text|summary → MATCH; filter|key → EQ/IN/NEQ/EXISTS;
// those plus comparable type → GT/GTE/LT/LTE; sort → SORT; stored is not a query face.
func AllowsOp(field IndexField, op SearchOp) bool {
	switch op {
	case OpMatch:
		return field.Has(HintText) || field.Has(HintSummary)
	case OpEQ, OpIN, OpNEQ, OpExists:
		return field.Has(HintFilter) || field.Has(HintKey)
	case OpGT, OpGTE, OpLT, OpLTE:
		return (field.Has(HintFilter) || field.Has(HintKey)) && ComparableType(field.Type)
	case OpSort:
		return field.Has(HintSort)
	default:
		return false
	}
}

// CheckSearch: each clause.Op must be implied by that path's AccessHints. Bare MATCH
// (no path) fans out across text fields. Failure is CAPABILITY_UNSATISFIED, not JSON contains.
func CheckSearch(req SearchRequest, spec IndexSpec) error {
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

func checkClause(c SearchClause, spec IndexSpec) error {
	if c.Op == OpMatch && c.Path == "" {
		if !spec.HasHint(HintText) && !spec.HasHint(HintSummary) {
			return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "MATCH needs a text or summary AccessHint")
		}
		return nil
	}
	if c.Path == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "%s requires path", c.Op)
	}
	if len(spec.Fields) == 0 {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "structured %s needs AccessHints; path %s is unplanned", c.Op, c.Path)
	}
	field, ok := spec.Field(c.Path)
	if !ok {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "path %s is not in IndexSpec", c.Path)
	}
	if !AllowsOp(field, c.Op) {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "%s is not implied by access %v type %q on %s", c.Op, field.Access, field.Type, c.Path)
	}
	return nil
}

func (s IndexSpec) Field(path string) (IndexField, bool) {
	for _, field := range s.Fields {
		if field.Path == path {
			return field, true
		}
	}
	return IndexField{}, false
}

func ComparableType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "int", "integer", "long", "number", "float", "double", "string", "date", "datetime", "timestamp":
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
