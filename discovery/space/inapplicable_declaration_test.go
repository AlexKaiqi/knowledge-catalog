package space_test

import (
	"strings"
	"testing"

	retrieval "kc/discovery"
	"kc/discovery/space"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

// A declaration selected into a Dataset whose input Schema was not selected
// has no applicable object there: queries on it miss completely instead of
// being reported as undeclared or unsupported.
func TestDatasetDeclarationOnUnselectedSchemaIsInapplicable(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://space/unselected-schema")
	base := testkit.MustHead(t, repo, snapshot.DefaultRef)
	at, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/table"}, PathHint: "tables/schema.yaml", Value: map[string]any{"entity": "Table", "fields": map[string]any{"name": map[string]any{"type": "string"}, "rowCount": map[string]any{"type": "number"}}}},
		testkit.IndexUnder("selected", testkit.IndexMember("table-name", "text", []string{objectmodel.IndexUseText}, "schema/table#/fields/name")),
		testkit.IndexUnder("selected", testkit.IndexMember("table-row-count", "number", []string{objectmodel.IndexUseFilter}, "schema/table#/fields/rowCount")),
	}})
	if err != nil {
		t.Fatal(err)
	}
	pin := reader.DatasetPin{SetID: "unselected-schema", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}, Items: []reader.DatasetItem{{Repository: repo.ID(), Commit: at, Kind: "prefix", Prefix: "selected"}}}
	view, err := space.Member(func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }, pin, repo.ID())
	if err != nil {
		t.Fatal(err)
	}
	spec, err := retrieval.AccessSpecForRepository(view, at)
	if err != nil {
		t.Fatal(err)
	}
	statuses := spec.Statuses()
	if len(statuses) != 2 {
		t.Fatalf("selected members: %#v", statuses)
	}
	for _, status := range statuses {
		if status.Status != retrieval.DeclarationInapplicable || !strings.Contains(status.Reason, "not in this scope") {
			t.Fatalf("unselected input Schema must make the member inapplicable: %#v", status)
		}
	}
	if members := spec.UnreadyTextMembers(); len(members) != 0 {
		t.Fatalf("inapplicable members must not degrade an unfielded MATCH: %#v", members)
	}
	five := "5"
	cases := map[string]retrieval.SearchRequest{
		"unfielded match": {Clauses: []retrieval.SearchClause{{Op: retrieval.OpMatch, Value: "orders"}}},
		"declared filter": {Clauses: []retrieval.SearchClause{{Op: retrieval.OpGTE, Path: "rowCount", Value: five}}},
		"all with filter": {Expression: &retrieval.SearchExpr{All: []retrieval.SearchExpr{{Clause: &retrieval.SearchClause{Op: retrieval.OpMatch, Value: "orders"}}, {Clause: &retrieval.SearchClause{Op: retrieval.OpGTE, Path: "rowCount", Value: five}}}}},
	}
	for name, request := range cases {
		if _, empty, err := retrieval.PruneInapplicable(request, spec); err != nil || !empty {
			t.Fatalf("%s: want a complete miss, got empty=%v err=%v", name, empty, err)
		}
	}
	typo := retrieval.SearchRequest{Clauses: []retrieval.SearchClause{{Op: retrieval.OpGTE, Path: "rowCount", Value: five}, {Op: retrieval.OpEQ, Path: "owner", Value: "x"}}}
	if _, _, err := retrieval.PruneInapplicable(typo, spec); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("an undeclared condition beside an inapplicable one must still fail: %v", err)
	}
	other := retrieval.SearchRequest{Clauses: []retrieval.SearchClause{{Op: retrieval.OpEQ, Path: "owner", Value: "x"}}}
	if _, empty, err := retrieval.PruneInapplicable(other, spec); err != nil || empty {
		t.Fatalf("a condition no member declares is left to normal resolution: empty=%v err=%v", empty, err)
	}
}
