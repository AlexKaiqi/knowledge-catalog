package dataset_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"kc/catalog"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestCatalogCompatibilityUsesTheDatasetTypeIdentity(t *testing.T) {
	for name, pair := range map[string][2]reflect.Type{
		"catalog":    {reflect.TypeOf(catalog.Catalog{}), reflect.TypeOf(dataset.Catalog{})},
		"registry":   {reflect.TypeOf(catalog.Registry{}), reflect.TypeOf(dataset.Registry{})},
		"definition": {reflect.TypeOf(catalog.KnowledgeSet{}), reflect.TypeOf(dataset.KnowledgeSet{})},
		"source":     {reflect.TypeOf(catalog.KnowledgeSetSource{}), reflect.TypeOf(dataset.KnowledgeSetSource{})},
		"pin":        {reflect.TypeOf(catalog.ResolvedKnowledgeSet{}), reflect.TypeOf(dataset.ResolvedKnowledgeSet{})},
	} {
		if pair[0] != pair[1] || pair[0].PkgPath() != "kc/snapshot/dataset" {
			t.Fatalf("%s owns a second type: %v and %v", name, pair[0], pair[1])
		}
	}
}

func TestCatalogCompatibilitySharesPublicationAuthority(t *testing.T) {
	source := newCatalogMem("kr://files/source")
	head, err := source.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("object_id: unchanged-content\nplain file bytes\n")
	commit, err := source.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: source.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: head, ExpectedTargetCommit: head,
		Changes: []snapshot.TreeChange{{Path: "source/original.txt", Content: body}},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := snapshot.NewRegistry()
	if err := store.Add(source); err != nil {
		t.Fatal(err)
	}
	authority := newCatalogMem("kr://files/catalog-authority")
	registry, err := catalog.CreateSnapshotRegistry(authority, "kr://files/catalog", "")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := catalog.NewCatalog(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.RegisterRepository(source.ID()); err != nil {
		t.Fatal(err)
	}
	definition, err := legacy.DefineKnowledgeSet("delivery", 1, []catalog.KnowledgeSetSource{{
		Repository: source.ID(), Selector: snapshot.DefaultRef,
		File: "source/original.txt", Target: "bundle/readme.txt",
	}})
	if err != nil {
		t.Fatal(err)
	}
	current, err := dataset.NewCatalog(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := current.DatasetVersion("delivery", 1)
	if err != nil || !reflect.DeepEqual(accepted, definition) {
		t.Fatalf("legacy publication was not recovered by the module: %#v, %v", accepted, err)
	}
	_, err = source.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: source.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: commit, ExpectedTargetCommit: commit,
		Changes: []snapshot.TreeChange{
			{Path: "source/original.txt", Remove: true},
			{Path: "source/renamed.txt", Content: []byte("replacement")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	pin, err := current.ResolveKnowledgeSet("delivery")
	if err != nil || len(pin.Items) != 1 {
		t.Fatalf("resolve published file: %#v, %v", pin, err)
	}
	item := pin.Items[0]
	if item.File != "source/original.txt" || item.Target != "bundle/readme.txt" || item.Commit != commit {
		t.Fatalf("file identity drifted after a source rename: %#v", item)
	}
	read, err := source.ReadFile(item.File, item.Commit)
	if err != nil || !bytes.Equal(read, body) {
		t.Fatalf("opaque file bytes changed: %q, %v", read, err)
	}
	if _, err := current.DefineKnowledgeSet("delivery", 2, accepted.Sources); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.DefineKnowledgeSet("delivery", 3, accepted.Sources); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("the old import path bypassed the shared authority CAS: %v", err)
	}
	reopened, err := catalog.NewCatalog(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := reopened.Set("delivery")
	if err != nil || latest.Revision != 2 {
		t.Fatalf("legacy consumer did not recover the current release: %#v, %v", latest, err)
	}
}

func TestCatalogCompatibilityPreservesFileJSON(t *testing.T) {
	const raw = `{"setId":"files","revision":1,"sources":[{"repository":"kr://files/source","selector":"refs/heads/main","commit":"fixed","file":"a.txt","target":"docs/a.txt"}],"items":[{"target":"docs/a.txt","repository":"kr://files/source","commit":"fixed","kind":"file","file":"a.txt"}]}`
	var legacy catalog.KnowledgeSet
	var current dataset.KnowledgeSet
	if err := catalog.DecodeJSON([]byte(raw), &legacy); err != nil {
		t.Fatal(err)
	}
	if err := dataset.DecodeJSON([]byte(raw), &current); err != nil {
		t.Fatal(err)
	}
	a, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(current)
	if err != nil || !bytes.Equal(a, b) || string(a) != raw {
		t.Fatalf("file definition wire shape changed: %s / %s, %v", a, b, err)
	}
	for _, invalid := range []string{
		`{"setId":"files","sources":[{"repository":"kr://files/source","objectId":"Entity:a"}]}`,
		`{"setId":"files","sources":[{"repository":"kr://files/source","aspect":"canonical"}]}`,
	} {
		if err := dataset.DecodeJSON([]byte(invalid), &current); err == nil {
			t.Fatalf("Dataset accepted a knowledge selector: %s", invalid)
		}
	}
}
