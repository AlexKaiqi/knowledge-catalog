package dataset_test

import (
	"reflect"
	"strings"
	"testing"

	catalog "kc/snapshot/dataset"
)

func TestKnowledgeSetRecipeRoundTripPreservesRootPath(t *testing.T) {
	def := catalog.KnowledgeSet{
		SetID: "notes",
		Sources: []catalog.KnowledgeSetSource{
			{Repository: "kr://acme/personals/alice", Selector: "refs/heads/main", Path: catalog.MountPath("")},
			{Repository: "kr://acme/public/semantic", Selector: "refs/heads/stable", Path: catalog.MountPath("refs/semantic"), SubPath: "metrics"},
		},
	}
	rec, ok := catalog.RecipeFromKnowledgeSet(def)
	if !ok || rec.Name != "notes" || len(rec.Mounts) != 2 {
		t.Fatalf("%#v %v", rec, ok)
	}
	if rec.Mounts[0].Path != "" {
		t.Fatalf("root mount path must be empty string in the file, got %q", rec.Mounts[0].Path)
	}
	raw, err := catalog.FormatKnowledgeSetRecipe(rec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "path: \"\"") && !strings.Contains(string(raw), "path: ''") {
		// yaml.v3 may emit `path: ""` or `path:`. Accept an explicit empty.
		if !strings.Contains(string(raw), "name: notes") {
			t.Fatalf("yaml: %s", raw)
		}
	}
	got, err := catalog.ParseKnowledgeSetRecipe(raw)
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
	def := catalog.KnowledgeSet{
		SetID: "agent",
		Sources: []catalog.KnowledgeSetSource{
			{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
		},
	}
	if _, ok := catalog.RecipeFromKnowledgeSet(def); ok {
		t.Fatal("federated-read workspaces must not emit a hitchhiking recipe file")
	}
}

func TestParseKnowledgeSetRecipeRequiresNameAndMounts(t *testing.T) {
	if _, err := catalog.ParseKnowledgeSetRecipe([]byte("mounts: []\n")); err == nil {
		t.Fatal("missing name")
	}
	if _, err := catalog.ParseKnowledgeSetRecipe([]byte("name: notes\nmounts: []\n")); err == nil {
		t.Fatal("empty mounts")
	}
}

func TestDatasetRecipeMetadataRoundTripAndIsolation(t *testing.T) {
	definition := catalog.KnowledgeSet{
		SetID: "delivery", Metadata: map[string]string{"description": "portable delivery", "custom": "42", "": ""},
		Sources: []catalog.KnowledgeSetSource{{Repository: "source", Selector: "refs/heads/main", File: "visible.txt", Target: "public.txt"}},
	}
	recipe, ok := catalog.RecipeFromKnowledgeSet(definition)
	if !ok {
		t.Fatal("file recipe not generated")
	}
	raw, err := catalog.FormatKnowledgeSetRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := catalog.ParseKnowledgeSetRecipe(raw)
	if err != nil || !reflect.DeepEqual(parsed.Metadata, definition.Metadata) || !reflect.DeepEqual(parsed.Sources(), definition.Sources) {
		t.Fatalf("portable metadata changed file selection: %#v %v", parsed, err)
	}
	recipe.Metadata["description"] = "recipe edit"
	if definition.Metadata["description"] != "portable delivery" {
		t.Fatal("portable recipe shares definition metadata")
	}
	if _, err := catalog.ParseKnowledgeSetRecipe([]byte("name: legacy\nmounts:\n  - repository: source\n    selector: refs/heads/main\n    path: ''\n")); err != nil {
		t.Fatalf("old recipe without metadata rejected: %v", err)
	}
}

func TestDatasetRecipeMetadataRequiresStringKeysAndValues(t *testing.T) {
	for _, metadata := range []string{
		"description: 42", "description: true", "description: null", "description: [text]",
		"description: {nested: value}", "42: value", "null", "[]", "description: first\n  description: second",
	} {
		raw := "name: delivery\nfiles:\n  - repository: source\n    selector: refs/heads/main\n    file: visible.txt\n    target: public.txt\nmetadata:\n  " + metadata + "\n"
		if _, err := catalog.ParseKnowledgeSetRecipe([]byte(raw)); err == nil {
			t.Fatalf("portable recipe accepted invalid string metadata: %s", metadata)
		}
	}
}
