package invocation

import (
	"testing"

	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestPublicCatalogViewHidesSelectors(t *testing.T) {
	view := InventoryView(dataset.State{
		Repositories: []string{"kr://acme/public/core", "kr://kc/system"},
		KnowledgeSets: []dataset.KnowledgeSet{{
			SetID:    "oncall",
			Revision: 1,
			Sources: []dataset.KnowledgeSetSource{
				{Repository: "kr://acme/public/core", Selector: snapshot.DefaultRef, Path: dataset.MountPath("knowledge")},
				{Repository: "kr://acme/public/core", Selector: snapshot.DefaultRef},
			},
		}},
	})
	if _, present := view["catalogId"]; present {
		t.Fatalf("%#v", view)
	}
	workspaces := view["datasets"].([]map[string]any)
	if len(workspaces) != 1 {
		t.Fatalf("%#v", view)
	}
	workspace := workspaces[0]
	if workspace["id"] != "oncall" || workspace["revision"] != 1 {
		t.Fatalf("%#v", workspace)
	}
	if _, ok := workspace["sources"]; ok {
		t.Fatalf("inventory must not expose sources: %#v", workspace)
	}
	if _, ok := workspace["selector"]; ok {
		t.Fatalf("inventory must not expose selectors: %#v", workspace)
	}
	repos := workspace["repositories"].([]string)
	if len(repos) != 1 || repos[0] != "kr://acme/public/core" {
		t.Fatalf("workspace inventory must list member ids once: %#v", workspace)
	}
}
