package cli

import (
	"testing"

	"kc/catalog"
	"kc/snapshot"
)

func TestWorkspaceSourcesFromDefaultsPublishedSelector(t *testing.T) {
	sources, err := workspaceSourcesFrom([]string{"kr://acme/public/core"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || string(sources[0].Repository) != "kr://acme/public/core" || sources[0].Selector != snapshot.DefaultRef {
		t.Fatalf("id-only --source must fill the published selector: %#v", sources)
	}

	emptyEquals, err := workspaceSourcesFrom([]string{"kr://acme/public/core="})
	if err != nil {
		t.Fatal(err)
	}
	if emptyEquals[0].Selector != snapshot.DefaultRef {
		t.Fatalf("empty selector must default: %#v", emptyEquals)
	}

	mounted, err := workspaceSourcesFrom([]string{"kr://acme/public/core@knowledge"})
	if err != nil {
		t.Fatal(err)
	}
	if mounted[0].Selector != snapshot.DefaultRef || mounted[0].Path == nil || *mounted[0].Path != "knowledge" {
		t.Fatalf("id@path must keep the mount without naming a ref: %#v", mounted)
	}

	explicit, err := workspaceSourcesFrom([]string{"kr://acme/public/core=refs/heads/stable@kb@docs"})
	if err != nil {
		t.Fatal(err)
	}
	if explicit[0].Selector != "refs/heads/stable" || explicit[0].Path == nil || *explicit[0].Path != "kb" || explicit[0].SubPath != "docs" {
		t.Fatalf("explicit selector and nested mount still parse: %#v", explicit)
	}

	if _, err := workspaceSourcesFrom(nil); err == nil {
		t.Fatal("missing --source must fail")
	}
}

func TestPublicCatalogViewHidesSelectors(t *testing.T) {
	view := publicCatalogView(catalog.CatalogState{
		CatalogID:    "kr://acme/catalog",
		Repositories: []string{"kr://acme/public/core", "kr://kc/system"},
		Workspaces: []catalog.WorkspaceDefinition{{
			WorkspaceID: "oncall",
			Revision:    1,
			Sources: []catalog.WorkspaceSource{
				{Repository: "kr://acme/public/core", Selector: snapshot.DefaultRef, Path: catalog.MountPath("knowledge")},
				{Repository: "kr://acme/public/core", Selector: snapshot.DefaultRef},
			},
		}},
	})
	if view["catalogId"] != "kr://acme/catalog" {
		t.Fatalf("%#v", view)
	}
	workspaces := view["workspaces"].([]map[string]any)
	if len(workspaces) != 1 {
		t.Fatalf("%#v", view)
	}
	workspace := workspaces[0]
	if workspace["workspaceId"] != "oncall" || workspace["revision"] != 1 {
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
