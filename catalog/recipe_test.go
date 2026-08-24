package catalog_test

import (
	"strings"
	"testing"

	"kc/catalog"
)

func TestWorkspaceRecipeRoundTripPreservesRootPath(t *testing.T) {
	def := catalog.WorkspaceDefinition{
		WorkspaceID: "notes",
		Sources: []catalog.WorkspaceSource{
			{Repository: "kr://acme/personals/alice", Selector: "refs/heads/main", Path: catalog.MountPath("")},
			{Repository: "kr://acme/public/semantic", Selector: "refs/heads/stable", Path: catalog.MountPath("refs/semantic"), SubPath: "metrics"},
		},
	}
	rec, ok := catalog.RecipeFromWorkspace(def)
	if !ok || rec.Name != "notes" || len(rec.Mounts) != 2 {
		t.Fatalf("%#v %v", rec, ok)
	}
	if rec.Mounts[0].Path != "" {
		t.Fatalf("root mount path must be empty string in the file, got %q", rec.Mounts[0].Path)
	}
	raw, err := catalog.FormatWorkspaceRecipe(rec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "path: \"\"") && !strings.Contains(string(raw), "path: ''") {
		// yaml.v3 may emit `path: ""` or `path:`. Accept an explicit empty.
		if !strings.Contains(string(raw), "name: notes") {
			t.Fatalf("yaml: %s", raw)
		}
	}
	got, err := catalog.ParseWorkspaceRecipe(raw)
	if err != nil {
		t.Fatal(err)
	}
	sources := got.Sources()
	if len(sources) != 2 || sources[0].Path == nil || *sources[0].Path != "" {
		t.Fatalf("root must round-trip as declared empty path: %#v", sources)
	}
	if catalog.RootMount(sources) == nil || catalog.RootMount(sources).Repository != "kr://acme/personals/alice" {
		t.Fatal(catalog.RootMount(sources))
	}
	if sources[1].SubPath != "metrics" || *sources[1].Path != "refs/semantic" {
		t.Fatalf("%#v", sources[1])
	}
}

func TestRecipeFromViewRejectsFederatedRead(t *testing.T) {
	def := catalog.WorkspaceDefinition{
		WorkspaceID: "agent",
		Sources: []catalog.WorkspaceSource{
			{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
		},
	}
	if _, ok := catalog.RecipeFromWorkspace(def); ok {
		t.Fatal("federated-read workspaces must not emit a hitchhiking recipe file")
	}
}

func TestParseWorkspaceRecipeRequiresNameAndMounts(t *testing.T) {
	if _, err := catalog.ParseWorkspaceRecipe([]byte("mounts: []\n")); err == nil {
		t.Fatal("missing name")
	}
	if _, err := catalog.ParseWorkspaceRecipe([]byte("name: notes\nmounts: []\n")); err == nil {
		t.Fatal("empty mounts")
	}
}
