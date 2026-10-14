package dataset_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func TestDatasetCatalogsSharePublicationAuthority(t *testing.T) {
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
	registry, err := catalog.CreateSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := catalog.NewService(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.RegisterRepository(source.ID()); err != nil {
		t.Fatal(err)
	}
	definition, err := publisher.DefineDataset("delivery", 1, []catalog.DatasetSource{{
		Repository: source.ID(), Selector: snapshot.DefaultRef,
		File: "source/original.txt", Target: "bundle/readme.txt",
	}})
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := catalog.NewService(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := consumer.DatasetVersion("delivery", 1)
	if err != nil || !reflect.DeepEqual(accepted, definition) {
		t.Fatalf("accepted publication was not recovered by the module: %#v, %v", accepted, err)
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
	pin, err := consumer.ResolveDataset("delivery")
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
	if _, err := consumer.DefineDataset("delivery", 2, accepted.Sources); err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.DefineDataset("delivery", 3, accepted.Sources); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("a stale Service bypassed the shared authority CAS: %v", err)
	}
	reopened, err := catalog.NewService(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := reopened.Set("delivery")
	if err != nil || latest.Revision != 2 {
		t.Fatalf("reopened Service did not recover the current release: %#v, %v", latest, err)
	}
}

func TestDatasetPreservesFileJSON(t *testing.T) {
	const raw = `{"setId":"files","revision":1,"sources":[{"repository":"kr://files/source","selector":"refs/heads/main","commit":"fixed","file":"a.txt","target":"docs/a.txt"}],"items":[{"target":"docs/a.txt","repository":"kr://files/source","commit":"fixed","kind":"file","file":"a.txt"}]}`
	var definition catalog.Dataset
	if err := catalog.DecodeJSON([]byte(raw), &definition); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(definition)
	if err != nil || string(body) != raw {
		t.Fatalf("file definition wire shape changed: %s, %v", body, err)
	}
	for _, invalid := range []string{
		`{"setId":"files","sources":[{"repository":"kr://files/source","objectId":"Entity:a"}]}`,
		`{"setId":"files","sources":[{"repository":"kr://files/source","aspect":"canonical"}]}`,
	} {
		if err := catalog.DecodeJSON([]byte(invalid), &definition); err == nil {
			t.Fatalf("Dataset accepted an object selector: %s", invalid)
		}
	}
}
