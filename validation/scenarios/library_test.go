package scenarios

import (
	"reflect"
	"strings"
	"testing"
)

func TestEmbeddedLibrary(t *testing.T) {
	library, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := library.Plan("all")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, scenario := range plan {
		got = append(got, scenario.ID)
	}
	want := []string{
		"fixture.tpch.oracle",
		"producer.mysql.structure",
		"producer.mysql.observations",
		"consumer.capture.tpch-view",
		"producer.mysql.cdc",
		"consumer.compare.tpch-pins",
		"producer.mysql.structure-auto",
		"knowledge.realistic.warehouse-graph",
		"index.declarative.publish-search",
		"index.user-publish.auto-refresh",
		"workspace.company.lifecycle",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("full plan\n got: %#v\nwant: %#v", got, want)
	}
}

func TestPlanDeduplicatesSharedAtoms(t *testing.T) {
	library := &Library{
		Version: 1,
		Fixture: Fixture{ID: "fixture"},
		Scenarios: []Scenario{
			{ID: "a", Kind: "action", Title: "a", Roles: []string{"r"}, Command: []string{"true"}, Assertions: []string{"a"}},
			{ID: "b", Kind: "action", Title: "b", Roles: []string{"r"}, Needs: []string{"a"}, Command: []string{"true"}, Assertions: []string{"b"}},
			{ID: "journey", Kind: "journey", Title: "journey", Roles: []string{"r"}, Goal: "do work", Outcome: "work done", Steps: []string{"a", "b"}, Assertions: []string{"journey"}},
		},
	}
	if err := library.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := library.Plan("journey")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 2 || plan[0].ID != "a" || plan[1].ID != "b" {
		t.Fatalf("unexpected plan %#v", plan)
	}
}

func TestValidateRejectsCycle(t *testing.T) {
	library := &Library{
		Version: 1,
		Fixture: Fixture{ID: "fixture"},
		Scenarios: []Scenario{
			{ID: "a", Kind: "journey", Title: "a", Roles: []string{"r"}, Goal: "a", Outcome: "a", Steps: []string{"b"}, Assertions: []string{"a"}},
			{ID: "b", Kind: "journey", Title: "b", Roles: []string{"r"}, Goal: "b", Outcome: "b", Steps: []string{"a"}, Assertions: []string{"b"}},
		},
	}
	if err := library.Validate(); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}
