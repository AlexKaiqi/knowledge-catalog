package discovery

import (
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
)

func (s AccessSpec) selectDeclarationBuild(field AccessField, op SearchOp) (*declarationBuild, error) {
	if len(s.builds) == 0 {
		return nil, nil
	}
	var candidates []*declarationBuild
	var members []string
	for i := range s.builds {
		build := &s.builds[i]
		if !declarationAllows(*build, op) {
			continue
		}
		for _, input := range build.inputs {
			if input.Field == field.FieldRef {
				candidates = append(candidates, build)
				members = append(members, objectmodel.AddressLabel(build.member))
				break
			}
		}
	}
	if len(candidates) > 1 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "field %s has ambiguous members for %s: %s", displayFieldRef(field.FieldRef), op, strings.Join(members, ", "))
	}
	if len(candidates) == 0 {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "field %s has no declaration member for %s", displayFieldRef(field.FieldRef), op)
	}
	return candidates[0], nil
}

func declarationAllows(build declarationBuild, op SearchOp) bool {
	hint := requiredHint(op)
	if !declarationHasUse(build.declaration.Uses, hint) {
		return false
	}
	switch build.declaration.Build.Kind {
	case "text":
		return op == OpMatch
	case "keyword":
		return !rangeOp(op) && op != OpMatch
	case "number", "date":
		return op != OpPrefix && op != OpContains && op != OpMatch
	case "boolean":
		return !rangeOp(op) && op != OpPrefix && op != OpContains && op != OpMatch && op != OpSort
	}
	return false
}

func rangeOp(op SearchOp) bool { return op == OpGT || op == OpGTE || op == OpLT || op == OpLTE }

func declarationHasUse(uses []string, required string) bool {
	for _, use := range uses {
		if use == required {
			return true
		}
	}
	return false
}

func requiredHint(op SearchOp) string {
	if op == OpMatch {
		return objectmodel.IndexUseText
	}
	if op == OpSort {
		return objectmodel.IndexUseSort
	}
	return objectmodel.IndexUseFilter
}

// ResolveClauseField returns the selected member's scalar representation.
// The query still locates a Schema field; keyword construction can stringify
// a numeric input while number construction preserves its numeric operations.
func (s AccessSpec) ResolveClauseField(clause SearchClause) (AccessField, error) {
	if clause.Field == nil {
		return AccessField{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "query field is unresolved")
	}
	field, err := s.ResolveField(*clause.Field)
	if err != nil {
		return AccessField{}, err
	}
	build, err := s.selectDeclarationBuild(field, clause.Op)
	if err != nil {
		return AccessField{}, err
	}
	if build != nil && build.declaration.Build.Kind == "keyword" {
		field.Type = "string"
	}
	return field, nil
}
