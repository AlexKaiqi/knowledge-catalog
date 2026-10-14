package dataset_test

import (
	"kc/snapshot"
	"kc/snapshot/dataset"
	"testing"
)

func TestSourceMappingsDefaultSelectorAndMountCoordinates(t *testing.T) {
	sources, err := dataset.ParseSourceMappings([]string{"kr://acme/public/core"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || string(sources[0].Repository) != "kr://acme/public/core" || sources[0].Selector != snapshot.DefaultRef {
		t.Fatalf("id-only --source must fill the published selector: %#v", sources)
	}

	emptyEquals, err := dataset.ParseSourceMappings([]string{"kr://acme/public/core="})
	if err != nil {
		t.Fatal(err)
	}
	if emptyEquals[0].Selector != snapshot.DefaultRef {
		t.Fatalf("empty selector must default: %#v", emptyEquals)
	}

	mounted, err := dataset.ParseSourceMappings([]string{"kr://acme/public/core@object"})
	if err != nil {
		t.Fatal(err)
	}
	if mounted[0].Selector != snapshot.DefaultRef || mounted[0].Path == nil || *mounted[0].Path != "object" {
		t.Fatalf("id@path must keep the mount without naming a ref: %#v", mounted)
	}

	explicit, err := dataset.ParseSourceMappings([]string{"kr://acme/public/core=refs/heads/stable@kb@docs"})
	if err != nil {
		t.Fatal(err)
	}
	if explicit[0].Selector != "refs/heads/stable" || explicit[0].Path == nil || *explicit[0].Path != "kb" || explicit[0].SubPath != "docs" {
		t.Fatalf("explicit selector and nested mount still parse: %#v", explicit)
	}

	if _, err := dataset.ParseSourceMappings(nil); err == nil {
		t.Fatal("missing --source must fail")
	}
}

func TestSourceMappingsDistinguishUndeclaredRootAndFileDelivery(t *testing.T) {
	sources, err := dataset.ParseSourceMappings([]string{"docs", "docs@", "data=stable@data@tables"})
	if err != nil {
		t.Fatal(err)
	}
	if sources[0].Path != nil || sources[1].Path == nil || *sources[1].Path != "" || sources[2].Selector != "stable" || *sources[2].Path != "data" || sources[2].SubPath != "tables" {
		t.Fatalf("portable directory coordinates lost declared-root distinction: %#v", sources)
	}
	files, err := dataset.ParseFileMappings([]string{"docs@handbook/README.md@reference/index.md", "data=stable@tables/daily.csv@exports/tea.csv"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Selector != snapshot.DefaultRef || files[0].File != "handbook/README.md" || files[0].Target != "reference/index.md" || files[0].Path != nil || files[1].Selector != "stable" || files[1].Target != "exports/tea.csv" {
		t.Fatalf("portable single-file coordinates changed: %#v", files)
	}
	if empty, err := dataset.ParseFileMappings(nil); err != nil || len(empty) != 0 {
		t.Fatalf("an absent file selection added entries: %#v %v", empty, err)
	}
	for _, invalid := range []string{"", "docs", "docs@file", "docs@@target", "docs@file@", "@file@target"} {
		if _, err := dataset.ParseFileMappings([]string{invalid}); err == nil {
			t.Errorf("incomplete file mapping accepted: %q", invalid)
		}
	}
}
