package snapshot

import (
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestConditionalCommandStoreNeverRepairsMissingBucketDuringWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.db")
	store := NewBoltCommandStore(path)
	if err := store.Ready(); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket(commandBucket) }); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	entry := CommandEntry{CommandID: "new", Digest: "digest", Status: StatusPending}
	if matched, err := store.(ConditionalCommandStore).CompareAndSwap("new", nil, &entry); err == nil || matched {
		t.Fatalf("missing command bucket was reinitialized: matched=%v err=%v", matched, err)
	}
	db, err = bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(commandBucket) != nil {
			t.Fatal("failed update repaired bucket")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
