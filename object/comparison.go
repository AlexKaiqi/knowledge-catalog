package object

import (
	"sort"
	"strings"

	"kc/kernel"
)

// ComparisonUnit is one complete local unit from one fixed source. It contains
// no observation or resolved interpretation; field projection never trims Value.
type ComparisonUnit struct {
	UnitDeclaration
	Source ReadSource `json:"source"`
	Value  any        `json:"value"`
}

// ComparisonRead is server-owned input, not an authorization claim. The caller
// must prove every candidate and selected path readable before setting Complete.
type ComparisonRead struct {
	Basis    ReadBasis
	Units    []ComparisonUnit
	Complete bool
}

type ComparisonSide struct {
	Basis ReadBasis `json:"basis"`
	State ReadState `json:"state"`
}

type UnitPresenceChange string

const (
	UnitAdded   UnitPresenceChange = "ADDED"
	UnitRemoved UnitPresenceChange = "REMOVED"
	UnitEntered UnitPresenceChange = "ENTERED"
	UnitLeft    UnitPresenceChange = "LEFT"
)

// UnitDifference distinguishes local changes from delivery membership. Removed
// is proved only when both views select the whole same source authority.
type UnitDifference struct {
	Address            Address            `json:"address"`
	Before             *ComparisonUnit    `json:"before,omitempty"`
	After              *ComparisonUnit    `json:"after,omitempty"`
	Presence           UnitPresenceChange `json:"presence,omitempty"`
	ValueChanged       bool               `json:"valueChanged,omitempty"`
	DeclarationChanged bool               `json:"declarationChanged,omitempty"`
	SourceChanged      bool               `json:"sourceChanged,omitempty"`
	KindChanged        bool               `json:"kindChanged,omitempty"`
}

type ObjectComparison struct {
	Reference    string           `json:"reference"`
	From         ComparisonSide   `json:"from"`
	To           ComparisonSide   `json:"to"`
	ScopeChanged bool             `json:"scopeChanged"`
	Equal        *bool            `json:"equal,omitempty"`
	Changes      []UnitDifference `json:"changes"`
}

// CompareLocalReads compares only the supplied complete local units. Different
// logical scopes have no whole-object equality verdict, even if digests match.
// Coordinates and interpretation dependencies remain on each independent side.
func CompareLocalReads(from, to ComparisonRead) (ObjectComparison, error) {
	left, leftState, err := comparisonUnits(from)
	if err != nil {
		return ObjectComparison{}, err
	}
	right, rightState, err := comparisonUnits(to)
	if err != nil {
		return ObjectComparison{}, err
	}
	a, _ := ParseReference(from.Basis.Reference)
	b, _ := ParseReference(to.Basis.Reference)
	ar, _ := FormatReference(a)
	br, _ := FormatReference(b)
	if ar != br || kernel.CanonicalDigest(readShapeScope(from.Basis.Shape)) != kernel.CanonicalDigest(readShapeScope(to.Basis.Shape)) {
		return ObjectComparison{}, kernel.Fail(kernel.ErrUsageInvalid, "object comparison requires the same object and logical selection")
	}
	out := ObjectComparison{Reference: ar, From: ComparisonSide{Basis: from.Basis, State: leftState}, To: ComparisonSide{Basis: to.Basis, State: rightState}, ScopeChanged: leftState.Scope != rightState.Scope, Changes: []UnitDifference{}}
	if !out.ScopeChanged {
		equal := leftState.Digest == rightState.Digest
		out.Equal = &equal
	}
	keys := map[string]bool{}
	for key := range left {
		keys[key] = true
	}
	for key := range right {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		before, hasBefore := left[key]
		after, hasAfter := right[key]
		change := UnitDifference{}
		switch {
		case hasBefore && hasAfter:
			change.Address = before.Address
			change.KindChanged = before.Address.Kind != after.Address.Kind
			change.ValueChanged = before.Digest != after.Digest
			change.DeclarationChanged = before.DeclarationDigest != after.DeclarationDigest
			change.SourceChanged = before.Source.Repository != after.Source.Repository || before.Source.Binding != after.Source.Binding
			if !change.KindChanged && !change.ValueChanged && !change.DeclarationChanged && !change.SourceChanged {
				continue
			}
			change.Before, change.After = &before, &after
		case hasBefore:
			change.Address, change.Before, change.Presence = before.Address, &before, UnitLeft
			if wholeComparisonSource(from.Basis, before.Source) && wholeComparisonSource(to.Basis, before.Source) {
				change.Presence = UnitRemoved
			}
		case hasAfter:
			change.Address, change.After, change.Presence = after.Address, &after, UnitEntered
			if wholeComparisonSource(from.Basis, after.Source) && wholeComparisonSource(to.Basis, after.Source) {
				change.Presence = UnitAdded
			}
		}
		out.Changes = append(out.Changes, change)
	}
	return out, nil
}

func comparisonUnits(input ComparisonRead) (map[string]ComparisonUnit, ReadState, error) {
	scope, err := input.Basis.ScopeDigest()
	if err != nil {
		return nil, ReadState{}, err
	}
	if !input.Complete {
		return nil, ReadState{}, kernel.Fail(kernel.ErrPreconditionFailed, "object comparison requires complete local reads on both sides")
	}
	ref, _ := ParseReference(input.Basis.Reference)
	sources := map[kernel.RepositoryID]ReadSource{}
	for _, source := range input.Basis.Candidates {
		sources[source.Repository] = source
	}
	units := map[string]ComparisonUnit{}
	declarations := make([]UnitDeclaration, 0, len(input.Units))
	assembly := make([]Unit, 0, len(input.Units))
	for _, unit := range input.Units {
		expected, present := sources[unit.Source.Repository]
		if !present || expected != unit.Source || unit.Address.ObjectID != ref.Object {
			return nil, ReadState{}, kernel.Fail(kernel.ErrPreconditionFailed, "comparison unit changed fixed source or object identity")
		}
		if err := AssertWritable(unit.Address); err != nil {
			return nil, ReadState{}, err
		}
		if unit.Digest != kernel.CanonicalDigest(unit.Value) || unit.DeclarationDigest != DeclarationDigest(unit.SchemaRef, unit.ValueSource) {
			return nil, ReadState{}, kernel.Fail(kernel.ErrPreconditionFailed, "comparison unit has invalid complete local digests")
		}
		key := AddressKey(unit.Address)
		if _, duplicate := units[key]; duplicate {
			return nil, ReadState{}, kernel.Fail(kernel.ErrObjectIDConflict, "comparison contains duplicate maintenance unit %s", AddressLabel(unit.Address))
		}
		units[key] = unit
		declarations = append(declarations, unit.UnitDeclaration)
		assembly = append(assembly, Unit{ObjectID: ref.Object, Address: unit.Address, Value: unit.Value, SchemaRef: unit.SchemaRef, ValueSource: unit.ValueSource, Digest: unit.Digest})
	}
	if _, err := AssembleUnits(assembly); err != nil {
		return nil, ReadState{}, err
	}
	return units, ReadState{Algorithm: LocalStateAlgorithm, Scope: scope, Digest: LocalStateDigest(declarations), Complete: true}, nil
}

func wholeComparisonSource(basis ReadBasis, source ReadSource) bool {
	present := false
	for _, candidate := range basis.Candidates {
		if candidate.Repository == source.Repository && candidate.Binding == source.Binding {
			present = true
		}
	}
	if !present {
		return false
	}
	for _, path := range basis.Paths {
		if path.Repository == source.Repository && path.Kind == "prefix" && strings.Trim(path.Prefix, "/") == "" {
			return true
		}
	}
	return false
}
