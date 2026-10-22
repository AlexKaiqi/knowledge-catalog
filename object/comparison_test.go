package object

import (
	"testing"

	"kc/kernel"
)

func comparisonFixture(commit kernel.CommitID) (ComparisonRead, ComparisonUnit) {
	source := ReadSource{Repository: "kr://comparison/source", Commit: commit, Binding: "source-binding"}
	basis := ReadBasis{Version: 1, Reference: "urn:kc:object:table/orders", Candidates: []ReadSource{source}, Paths: []ReadPath{{Repository: source.Repository, Kind: "prefix"}}}
	value := map[string]any{"name": "orders", "owner": "trade"}
	unit := Unit{ObjectID: "table/orders", Address: Address{Kind: KindAspect, ObjectID: "table/orders", AspectName: "definition"}, Value: value, Digest: kernel.CanonicalDigest(value), SchemaRef: "schema/table"}
	declared := DeclareUnits([]Unit{unit})[0]
	row := ComparisonUnit{UnitDeclaration: declared, Source: source, Value: value}
	return ComparisonRead{Basis: basis, Units: []ComparisonUnit{row}, Complete: true}, row
}

func TestComparisonSeparatesLocalValueDeclarationKindAndVersionCoordinates(t *testing.T) {
	before, _ := comparisonFixture("a")
	for _, test := range []struct {
		name              string
		mutate            func(*ComparisonRead)
		value, decl, kind bool
	}{
		{name: "unrelated commit", mutate: func(input *ComparisonRead) {
			input.Basis.Candidates[0].Commit = "b"
			input.Units[0].Source.Commit = "b"
		}},
		{name: "unreturned field", value: true, mutate: func(input *ComparisonRead) {
			input.Units[0].Value = map[string]any{"name": "orders", "owner": "ops"}
			input.Units[0].Digest = kernel.CanonicalDigest(input.Units[0].Value)
		}},
		{name: "declaration", decl: true, mutate: func(input *ComparisonRead) {
			input.Units[0].SchemaRef = "schema/other"
			input.Units[0].DeclarationDigest = DeclarationDigest("schema/other", nil)
		}},
		{name: "kind", kind: true, mutate: func(input *ComparisonRead) { input.Units[0].Address.Kind = KindRecord }},
	} {
		t.Run(test.name, func(t *testing.T) {
			after, _ := comparisonFixture("a")
			test.mutate(&after)
			result, err := CompareLocalReads(before, after)
			changed := test.value || test.decl || test.kind
			if err != nil || result.Equal == nil || *result.Equal == changed || result.ScopeChanged {
				t.Fatalf("wrong whole-state verdict: %#v %v", result, err)
			}
			if changed {
				if len(result.Changes) != 1 || result.Changes[0].ValueChanged != test.value || result.Changes[0].DeclarationChanged != test.decl || result.Changes[0].KindChanged != test.kind {
					t.Fatalf("wrong local delta: %#v", result.Changes)
				}
			} else if len(result.Changes) != 0 {
				t.Fatalf("coordinates were mistaken for local change: %#v", result.Changes)
			}
		})
	}
}

func TestComparisonScopeChangesAndFileSlicesNeverInventSourceDeletion(t *testing.T) {
	before, _ := comparisonFixture("a")
	absent, _ := comparisonFixture("b")
	absent.Units = nil
	removed, err := CompareLocalReads(before, absent)
	if err != nil || len(removed.Changes) != 1 || removed.Changes[0].Presence != UnitRemoved || removed.Equal == nil || *removed.Equal {
		t.Fatalf("whole-source absence not proved: %#v %v", removed, err)
	}
	for _, sameScope := range []bool{true, false} {
		a, _ := comparisonFixture("a")
		b, _ := comparisonFixture("b")
		b.Units = nil
		a.Basis.Paths[0].Prefix = "public"
		b.Basis.Paths[0].Prefix = "public"
		if !sameScope {
			b.Basis.Paths[0].Prefix = "different"
		}
		result, err := CompareLocalReads(a, b)
		if err != nil || len(result.Changes) != 1 || result.Changes[0].Presence != UnitLeft {
			t.Fatalf("slice absence was called source deletion: %#v %v", result, err)
		}
		if !sameScope && (result.Equal != nil || !result.ScopeChanged) {
			t.Fatalf("cross-scope equality was invented: %#v", result)
		}
	}
	after, _ := comparisonFixture("b")
	after.Basis.Candidates = append(after.Basis.Candidates, ReadSource{Repository: "kr://comparison/empty", Commit: "e", Binding: "empty-binding"})
	after.Basis.Paths = append(after.Basis.Paths, ReadPath{Repository: "kr://comparison/empty", Kind: "prefix"})
	result, err := CompareLocalReads(before, after)
	if err != nil || !result.ScopeChanged || result.Equal != nil || len(result.Changes) != 0 {
		t.Fatalf("empty candidate disappeared from comparison scope: %#v %v", result, err)
	}
}

func TestComparisonRejectsIncompleteConflictingOrMismatchedInputs(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ComparisonRead)
		code   kernel.ErrorCode
	}{
		{"incomplete", func(input *ComparisonRead) { input.Complete = false }, kernel.ErrPreconditionFailed},
		{"absent source", func(input *ComparisonRead) { input.Units[0].Source = ReadSource{} }, kernel.ErrPreconditionFailed},
		{"wrong commit", func(input *ComparisonRead) { input.Units[0].Source.Commit = "other" }, kernel.ErrPreconditionFailed},
		{"wrong digest", func(input *ComparisonRead) { input.Units[0].Digest = "invalid" }, kernel.ErrPreconditionFailed},
		{"conflicting unit", func(input *ComparisonRead) { input.Units = append(input.Units, input.Units[0]) }, kernel.ErrObjectIDConflict},
		{"different object", func(input *ComparisonRead) { input.Basis.Reference = "urn:kc:object:table/other"; input.Units = nil }, kernel.ErrUsageInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, _ := comparisonFixture("a")
			after, _ := comparisonFixture("b")
			test.change(&after)
			if _, err := CompareLocalReads(before, after); kernel.CodeOf(err) != test.code {
				t.Fatalf("invalid comparison accepted: %v", err)
			}
		})
	}
}

func TestComparisonKeepsCompositionAndSourceChangesDistinctFromValues(t *testing.T) {
	before, _ := comparisonFixture("a")
	after, row := comparisonFixture("b")
	after.Units = nil
	for _, item := range []struct{ key, value string }{{"name", "orders"}, {"owner", "trade"}} {
		unit := row
		unit.Address.Kind = KindMember
		unit.Address.MemberKey = item.key
		unit.Value = item.value
		unit.Digest = kernel.CanonicalDigest(item.value)
		after.Units = append(after.Units, unit)
	}
	result, err := CompareLocalReads(before, after)
	if err != nil || result.Equal == nil || *result.Equal || len(result.Changes) != 3 {
		t.Fatalf("equal assembled value hid unit split: %#v %v", result, err)
	}
	added, removed := 0, 0
	for _, change := range result.Changes {
		if change.Presence == UnitAdded {
			added++
		}
		if change.Presence == UnitRemoved {
			removed++
		}
	}
	if added != 2 || removed != 1 {
		t.Fatalf("unit split did not preserve addresses: %#v", result.Changes)
	}
	changed, _ := comparisonFixture("b")
	changed.Basis.Candidates[0].Binding = "another-authority"
	changed.Units[0].Source.Binding = "another-authority"
	result, err = CompareLocalReads(before, changed)
	if err != nil || result.Equal != nil || !result.ScopeChanged || len(result.Changes) != 1 || !result.Changes[0].SourceChanged || result.Changes[0].ValueChanged || result.Changes[0].DeclarationChanged {
		t.Fatalf("new source masqueraded as value change: %#v %v", result, err)
	}
}

func TestComparisonBindingUsesLocalDeclarationAndIgnoresInterpretationCoordinates(t *testing.T) {
	makeRead := func(commit kernel.CommitID, descriptor ObjectID) ComparisonRead {
		input, _ := comparisonFixture(commit)
		unit := &input.Units[0]
		unit.Value = nil
		unit.Digest = kernel.CanonicalDigest(nil)
		unit.ValueSource = &ValueSource{Kind: ValueSourceBinding, Binding: &BindingDeclaration{DescriptorRef: descriptor}}
		unit.DeclarationDigest = DeclarationDigest(unit.SchemaRef, unit.ValueSource)
		input.Basis.Dependencies = []ReadSource{{Repository: "kr://comparison/descriptor", Commit: commit, Binding: "descriptor-authority"}}
		return input
	}
	before := makeRead("a", "resource/orders")
	after := makeRead("b", "resource/orders")
	result, err := CompareLocalReads(before, after)
	if err != nil || result.Equal == nil || !*result.Equal || len(result.Changes) != 0 {
		t.Fatalf("interpretation version became local state: %#v %v", result, err)
	}
	after = makeRead("b", "resource/other")
	result, err = CompareLocalReads(before, after)
	if err != nil || result.Equal == nil || *result.Equal || len(result.Changes) != 1 || !result.Changes[0].DeclarationChanged || result.Changes[0].ValueChanged {
		t.Fatalf("binding declaration change was lost in nil values: %#v %v", result, err)
	}
}
