package invocation

import (
	"kc/invoke"
	"kc/kernel"
	"testing"
)

func TestLimitFlagRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"abc", "-1", "1.5"} {
		if _, err := LimitFrom(map[string]invoke.FlagValue{"limit": raw}, 50); err == nil {
			t.Errorf("--limit %s should be rejected", raw)
		}
	}
	got, err := LimitFrom(map[string]invoke.FlagValue{}, 50)
	if err != nil || got != 50 {
		t.Fatalf("absent --limit: %d %v", got, err)
	}
}

func TestPageLimitTreatsZeroAsDefaultAndRejectsOversized(t *testing.T) {
	got, err := PageLimit(map[string]invoke.FlagValue{"limit": "0"}, 50, 200)
	if err != nil || got != 50 {
		t.Fatalf("limit 0: %d %v", got, err)
	}
	got, err = PageLimit(map[string]invoke.FlagValue{}, 50, 200)
	if err != nil || got != 50 {
		t.Fatalf("omitted limit: %d %v", got, err)
	}
	if _, err := PageLimit(map[string]invoke.FlagValue{"limit": "201"}, 50, 200); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("oversized page: %v", err)
	}
}

func TestAddressCoordinatesRejectMemberWithoutAspect(t *testing.T) {
	if UsesAddress(map[string]invoke.FlagValue{"object": "policy/x"}) {
		t.Fatal("object-only is not an Address")
	}
	if !UsesAddress(map[string]invoke.FlagValue{"object": "policy/x", "aspect": "io"}) {
		t.Fatal("aspect is an Address")
	}
	if !UsesAddress(map[string]invoke.FlagValue{"object": "policy/x", "member": "user:bob"}) {
		t.Fatal("member without aspect is still an Address attempt")
	}
	if _, err := AddressFrom(map[string]invoke.FlagValue{"object": "policy/x", "member": "user:bob"}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("member without aspect: %v", err)
	}
}

func TestSortFlagKeepsFieldLocatorWithoutDirection(t *testing.T) {
	for raw, want := range map[string]string{
		"schema/table.live-stats::live-stats::rowCount":      "asc",
		"schema/table.live-stats::live-stats::rowCount:desc": "desc",
		"schema/table.live-stats::live-stats::rowCount:asc":  "asc",
	} {
		req, err := SearchRequestFromFlags(map[string]invoke.FlagValue{"sort": raw})
		if err != nil || len(req.Clauses) != 1 {
			t.Fatalf("%s: %#v %v", raw, req, err)
		}
		clause := req.Clauses[0]
		if clause.Field == nil || clause.Field.Path != "rowCount" || clause.Field.Aspect != "live-stats" || clause.Order != want {
			t.Fatalf("%s: field locator split as a direction: %#v", raw, clause)
		}
	}
}
