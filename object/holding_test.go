package object

import (
	"strings"
	"testing"

	"kc/kernel"
)

func holding(repo string, readable bool, aspects ...string) Holding {
	return Holding{Repository: kernel.RepositoryID(repo), Commit: kernel.CommitID("c-" + repo), Aspects: aspects, Readable: readable}
}

func TestSelectHoldingsComposesDisjointAspectsAcrossRepositories(t *testing.T) {
	got, err := SelectHoldings("table/orders", "", []Holding{holding("kr://a", true, "columns"), holding("kr://b", true, "readme")})
	if err != nil || len(got.Selected) != 2 || len(got.Withheld) != 0 {
		t.Fatalf("one ID in two Repositories must be one object with two Aspects: %#v %v", got, err)
	}
	located, err := SelectHoldings("table/orders", "/readme", []Holding{holding("kr://a", true, "columns"), holding("kr://b", true, "readme")})
	if err != nil || len(located.Selected) != 1 || located.Selected[0].Repository != "kr://b" {
		t.Fatalf("a locator must select only the holder of its Aspect: %#v %v", located, err)
	}
}

func TestSelectHoldingsReportsSameAspectFromTwoRepositories(t *testing.T) {
	_, err := SelectHoldings("table/orders", "", []Holding{holding("kr://a", true, "columns"), holding("kr://b", false, "columns")})
	if kernel.CodeOf(err) != kernel.ErrObjectIDConflict || !strings.Contains(err.Error(), "kr://a") {
		t.Fatalf("same Aspect from two Repositories must conflict: %v", err)
	}
	if strings.Contains(err.Error(), "kr://b") {
		t.Fatalf("conflict must not disclose an unreadable Repository name: %v", err)
	}
	whole := Holding{Repository: "kr://w", Commit: "c", WholeValue: true, Readable: true}
	if _, err := SelectHoldings("table/orders", "", []Holding{whole, holding("kr://a", true, "columns")}); kernel.CodeOf(err) != kernel.ErrObjectIDConflict {
		t.Fatalf("a whole value cannot be combined with another holder: %v", err)
	}
}

func TestSelectHoldingsWithholdsUnreadableAndRefusesWhenNothingIsReadable(t *testing.T) {
	got, err := SelectHoldings("table/orders", "", []Holding{holding("kr://a", true, "columns"), holding("kr://b", false, "readme")})
	if err != nil || len(got.Selected) != 1 || len(got.Withheld) != 1 || got.Withheld[0] != "kr://b" {
		t.Fatalf("unreadable holder must be marked Withheld: %#v %v", got, err)
	}
	if _, err := SelectHoldings("table/orders", "", []Holding{holding("kr://b", false, "readme")}); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("held only by unreadable Repositories must be FORBIDDEN: %v", err)
	}
}

func TestSelectHoldingsIgnoresRepositoriesThatHoldNothing(t *testing.T) {
	empty := Holding{Repository: "kr://system", Commit: "c", Readable: true}
	got, err := SelectHoldings("note/one", "", []Holding{empty, {Repository: "kr://docs", Commit: "c", WholeValue: true, Readable: true}})
	if err != nil || len(got.Selected) != 1 || got.Selected[0].Repository != "kr://docs" {
		t.Fatalf("a Repository with no units must not count as a holder: %#v %v", got, err)
	}
	if _, err := SelectHoldings("note/one", "", []Holding{empty}); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("no holder must be OBJECT_REF_UNRESOLVED: %v", err)
	}
	if _, err := SelectHoldings("note/one", "/missing", []Holding{holding("kr://a", true, "columns")}); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("an Aspect nobody holds must be OBJECT_REF_UNRESOLVED: %v", err)
	}
}

func TestComposeHoldingValuesRecordsVersionVector(t *testing.T) {
	values := []ObjectValue{
		{ObjectRef: ObjectRef{Repository: "kr://a", Object: "table/orders"}, Repository: "kr://a", Commit: "c1", Value: map[string]any{"columns": 1}},
		{ObjectRef: ObjectRef{Repository: "kr://b", Object: "table/orders"}, Repository: "kr://b", Commit: "c2", Value: map[string]any{"readme": "x"}},
	}
	got, err := ComposeHoldingValues("urn:kc:object:table/orders", values)
	if err != nil || len(got.Sources) != 2 || got.Sources[0].Repository != "kr://a" || got.Sources[1].Commit != "c2" || got.Digest == "" {
		t.Fatalf("composition must keep every source basis: %#v %v", got, err)
	}
	values[1].Value = map[string]any{"columns": 2}
	if _, err := ComposeHoldingValues("urn:kc:object:table/orders", values); kernel.CodeOf(err) != kernel.ErrObjectIDConflict {
		t.Fatalf("duplicate Aspect at composition time must conflict: %v", err)
	}
}
