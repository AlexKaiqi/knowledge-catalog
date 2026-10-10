package invocation

import (
	"kc/catalog"
	"kc/invoke"
	"kc/kernel"
	"reflect"
	"testing"
)

func TestDatasetManagementViewSelectsOnlyAcceptedHistory(t *testing.T) {
	old := catalog.KnowledgeSet{SetID: "shared", Revision: 1, Sources: []catalog.KnowledgeSetSource{{Repository: "kr://old", Commit: "c1"}}}
	current := catalog.KnowledgeSet{SetID: "shared", Revision: 3, Retired: true, Sources: []catalog.KnowledgeSetSource{{Repository: "kr://new", Commit: "c3"}}}
	state := catalog.CatalogState{KnowledgeSets: []catalog.KnowledgeSet{current}, DatasetVersions: []catalog.KnowledgeSet{old, current, {SetID: "other", Revision: 2}}}
	view, err := datasetManagementView(state, "shared", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if view["revision"] != 1 || view["servingRevision"] != 3 || view["retired"] != true || !reflect.DeepEqual(view["versions"], []int{1, 3}) {
		t.Fatalf("history lost current lifecycle or invented a version: %#v", view)
	}
	sources, ok := view["sources"].([]catalog.KnowledgeSetSource)
	if !ok || len(sources) != 1 || sources[0].Commit != "c1" {
		t.Fatalf("history source is not frozen: %#v", view)
	}
	_, err = datasetManagementView(state, "shared", 2, nil)
	if kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("unpublished v2: %v", err)
	}
	_, err = datasetManagementView(state, "other", 2, nil)
	if kernel.CodeOf(err) != kernel.ErrKnowledgeSetInvalid {
		t.Fatalf("historical record must not invent a visible current dataset: %v", err)
	}
	state.KnowledgeSets[0].Retired = false
	state.DatasetVersions[0].Retired = true
	view, err = datasetManagementView(state, "shared", 1, nil)
	if err != nil || view["retired"] == true {
		t.Fatalf("republished dataset kept a historical lifecycle verdict: %#v %v", view, err)
	}
}

func TestDatasetManagementViewPreservesLegacyAcceptedCurrentVersion(t *testing.T) {
	view, err := datasetManagementView(catalog.CatalogState{KnowledgeSets: []catalog.KnowledgeSet{{SetID: "legacy", Revision: 7}}}, "legacy", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(view["versions"], []int{7}) || view["servingRevision"] != 7 {
		t.Fatalf("legacy release fabricated history: %#v", view)
	}
}

func TestDatasetManagementHistoryPagesAcceptedRevisionsWithHardLimit(t *testing.T) {
	state := catalog.CatalogState{CatalogID: "kr://history", KnowledgeSets: []catalog.KnowledgeSet{{SetID: "shared", Revision: 501}}}
	// Accepted releases deliberately contain gaps and a duplicate current
	// record, as durable CatalogState does. Neither creates a synthetic version.
	for revision := 1; revision <= 501; revision += 2 {
		state.DatasetVersions = append(state.DatasetVersions, catalog.KnowledgeSet{SetID: "shared", Revision: revision})
	}
	for _, limit := range []string{"", "0"} {
		view, err := datasetManagementView(state, "shared", 1, map[string]invoke.FlagValue{"limit": limit})
		if err != nil {
			t.Fatal(err)
		}
		versions := view["versions"].([]int)
		if len(versions) != DefaultHistoryLimit || versions[0] != 403 || versions[len(versions)-1] != 501 || view["revision"] != 1 || view["continuation"] == "" {
			t.Fatalf("default history page is unbounded or point lookup changed: %#v", view)
		}
	}
	view, err := datasetManagementView(state, "shared", 0, map[string]invoke.FlagValue{"limit": MaxHistoryPageSize})
	if err != nil || len(view["versions"].([]int)) != MaxHistoryPageSize {
		t.Fatalf("maximum accepted page: %#v %v", view, err)
	}
	page, err := datasetManagementView(state, "shared", 0, map[string]invoke.FlagValue{"continuation": view["continuation"]})
	if err != nil || !reflect.DeepEqual(page["versions"], oddRevisions(3, 101)) {
		t.Fatalf("continuation lost accepted releases: %#v %v", page, err)
	}
	last, err := datasetManagementView(state, "shared", 0, map[string]invoke.FlagValue{"continuation": page["continuation"]})
	if err != nil || !reflect.DeepEqual(last["versions"], []int{1}) || last["continuation"] != nil {
		t.Fatalf("final history page should exhaust without invented revisions: %#v %v", last, err)
	}
	for _, limit := range []string{"-1", "nope", "201"} {
		_, err := datasetManagementView(state, "shared", 0, map[string]invoke.FlagValue{"limit": limit})
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("history limit %q: %v", limit, err)
		}
	}
}

func oddRevisions(start, end int) []int {
	result := []int{}
	for revision := start; revision <= end; revision += 2 {
		result = append(result, revision)
	}
	return result
}

func TestDatasetManagementHistoryContinuationRejectsChangedBasis(t *testing.T) {
	state := catalog.CatalogState{CatalogID: "kr://history", KnowledgeSets: []catalog.KnowledgeSet{{SetID: "shared", Revision: 3}}, DatasetVersions: []catalog.KnowledgeSet{{SetID: "shared", Revision: 1}}}
	view, err := datasetManagementView(state, "shared", 0, map[string]invoke.FlagValue{"limit": 1})
	if err != nil {
		t.Fatal(err)
	}
	raw := view["continuation"].(string)
	for _, tc := range []struct {
		catalog string
		dataset string
		serving int
	}{
		{"kr://other", "shared", 3}, {"kr://history", "other", 3}, {"kr://history", "shared", 4},
	} {
		_, err := decodeDatasetHistoryCursor(raw, tc.catalog, tc.dataset, tc.serving)
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("continuation accepted changed basis: %#v %v", tc, err)
		}
	}
	_, err = datasetManagementView(state, "shared", 0, map[string]invoke.FlagValue{"continuation": "not-base64"})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("malformed continuation: %v", err)
	}
	state.KnowledgeSets[0].Revision = 4
	_, err = datasetManagementView(state, "shared", 0, map[string]invoke.FlagValue{"continuation": raw})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("published update changed paging basis without rejection: %v", err)
	}
}
