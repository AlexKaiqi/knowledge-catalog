package testkit

import (
	"bytes"
	"testing"

	"kc/snapshot"
)

func TestLakeFSFakeBulkPublishesExactBytesAndKeepsOriginalVersion(t *testing.T) {
	fake := NewLakeFSFake(t)
	repo, err := snapshot.OpenLakeFS("kr://testkit/bulk", fake.DSN(fake.NewRepo()), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	root, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte{0, 1, 2, '\n', 255}
	published, err := repo.(snapshot.BulkTreeIngester).ApplyTreeCommitBulk(snapshot.TreeChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root, Changes: []snapshot.TreeChange{{Path: "data/raw.bin", Content: body}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := repo.(snapshot.TreeReader).ReadFile("data/raw.bin", published)
	if err != nil || !bytes.Equal(raw, body) {
		t.Fatalf("bulk round trip: %v %v", raw, err)
	}
	if !repo.HasCommit(root) || published == root {
		t.Fatal("bulk did not retain immutable prior version")
	}
	if _, err := repo.(snapshot.TreeReader).ReadFile("data/raw.bin", root); err == nil {
		t.Fatal("bulk changed original tree")
	}
}
