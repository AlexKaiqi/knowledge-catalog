package datasetbridge_test

import (
	"reflect"
	"testing"

	"kc/internal/datasetbridge"
	"kc/kernel"
	"kc/object/reader"
	"kc/snapshot/dataset"
)

// The expectations below are written by hand from the two public shapes. They
// are deliberately not produced by calling the conversion, so a field silently
// dropped inside the conversion cannot also disappear from the expectation.
func TestPinCarriesEveryConsumerField(t *testing.T) {
	resolved := dataset.ResolvedDataset{
		SetID:    "qinghe-tea",
		Revision: 7,
		Ref:      "v7",
		PinID:    "pin-abc",
		Repositories: map[kernel.RepositoryID]kernel.CommitID{
			"orders": "c0ffee",
			"menus":  "beef01",
		},
		Items: []dataset.DatasetItem{
			{Target: "orders/2024/tea.csv", Repository: "orders", Commit: "c0ffee", Kind: "file", File: "2024/tea.csv"},
			{Target: "menus", Repository: "menus", Commit: "beef01", Kind: "prefix", Prefix: "menus/"},
		},
		Dependencies: map[kernel.RepositoryID]kernel.CommitID{
			"schemas": "5chema",
		},
	}
	want := reader.DatasetPin{
		SetID:    "qinghe-tea",
		Revision: 7,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{
			"orders": "c0ffee",
			"menus":  "beef01",
		},
		Items: []reader.DatasetItem{
			{Target: "orders/2024/tea.csv", Repository: "orders", Commit: "c0ffee", Kind: "file", File: "2024/tea.csv"},
			{Target: "menus", Repository: "menus", Commit: "beef01", Kind: "prefix", Prefix: "menus/"},
		},
		Dependencies: map[kernel.RepositoryID]kernel.CommitID{"schemas": "5chema"},
	}
	if got := datasetbridge.Pin(resolved); !reflect.DeepEqual(got, want) {
		t.Fatalf("pin lost or invented consumer fields\n got: %#v\nwant: %#v", got, want)
	}
}

// A dependency is an interpretation version, never a member: it must not appear
// in Repositories or Items, or a blocked Repository would become readable.
func TestPinKeepsDependenciesOutOfMembership(t *testing.T) {
	pin := datasetbridge.Pin(dataset.ResolvedDataset{
		SetID:        "qinghe-tea",
		Revision:     1,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{"orders": "c0ffee"},
		Dependencies: map[kernel.RepositoryID]kernel.CommitID{"schemas": "5chema"},
	})
	if _, member := pin.Repositories["schemas"]; member {
		t.Fatal("dependency became a selected member repository")
	}
	for _, item := range pin.Items {
		if item.Repository == "schemas" {
			t.Fatal("dependency became a selected item")
		}
	}
	if pin.Dependencies["schemas"] != "5chema" {
		t.Fatal("dependency version was not frozen onto the pin")
	}
}

// An empty selection stays empty: no items and no repositories is a valid pin
// and must not be widened into "read the whole Repository".
func TestPinDoesNotWidenEmptySelection(t *testing.T) {
	pin := datasetbridge.Pin(dataset.ResolvedDataset{SetID: "empty", Revision: 3})
	if len(pin.Items) != 0 || len(pin.Repositories) != 0 || len(pin.Dependencies) != 0 {
		t.Fatalf("empty selection gained scope: %#v", pin)
	}
	if pin.SetID != "empty" || pin.Revision != 3 {
		t.Fatalf("empty selection lost its identity: %#v", pin)
	}
}

// Converting a frozen resolution twice must not follow a later HEAD: the pin is
// a copy of the coordinates it was given, not a live lookup.
func TestPinIsAStableCopy(t *testing.T) {
	resolved := dataset.ResolvedDataset{
		SetID:        "qinghe-tea",
		Revision:     2,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{"orders": "c0ffee"},
		Items:        []dataset.DatasetItem{{Target: "orders/a", Repository: "orders", Commit: "c0ffee", Kind: "file", File: "a"}},
	}
	pin := datasetbridge.Pin(resolved)
	resolved.Repositories["orders"] = "1ater"
	resolved.Items[0].Commit = "1ater"
	if pin.Repositories["orders"] != "c0ffee" || pin.Items[0].Commit != "c0ffee" {
		t.Fatal("pin followed a source that advanced after conversion")
	}
}
