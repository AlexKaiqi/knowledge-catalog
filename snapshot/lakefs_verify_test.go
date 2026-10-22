package snapshot_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

func TestLakeFSNoChangeVerificationCannotBorrowHeldPublicationLock(t *testing.T) {
	fake := newFakeLakeFS(t)
	repo := fake.open(t, "kr://conformance/no-change-lock")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	base, err := repo.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root, Changes: []snapshot.TreeChange{{Path: "file.txt", Content: []byte("same")}}})
	if err != nil {
		t.Fatal(err)
	}
	lock := "kc-publish-lock-" + string(kernel.CanonicalDigest(string(repo.ID()) + "\x00main"))[:24]
	fake.mu.Lock()
	fake.branches[lock] = string(base)
	before := len(fake.commits)
	fake.mu.Unlock()
	verifier := any(repo).(snapshot.TreeWriteVerifier)
	cs := snapshot.TreeChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Changes: []snapshot.TreeChange{{Path: "file.txt", Content: []byte("same")}}}
	err = verifier.VerifyTreeWrite(cs, false)
	testkit.ExpectCode(t, err, kernel.ErrTemporaryUnavailable)
	if !kernel.IsWriteNotApplied(err) {
		t.Fatalf("verifier lost non-publication proof: %v", err)
	}
	fake.mu.Lock()
	held := fake.branches[lock]
	commits := len(fake.commits)
	fake.mu.Unlock()
	if held != string(base) || commits != before || testkit.MustHead(t, repo, snapshot.DefaultRef) != base {
		t.Fatal("verification borrowed/deleted another publisher's lock or created commit")
	}
	if err := snapshot.RecoverLakeFSPublication(repo, snapshot.DefaultRef, base, base); err != nil {
		t.Fatal(err)
	}
	if err := verifier.VerifyTreeWrite(cs, false); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	_, retained := fake.branches[lock]
	commits = len(fake.commits)
	fake.mu.Unlock()
	if retained || commits != before || testkit.MustHead(t, repo, snapshot.DefaultRef) != base {
		t.Fatal("successful verifier created commit, moved ref or retained own lock")
	}
}

func TestLakeFSAuthorityOriginIncludesPhysicalRepository(t *testing.T) {
	fake := testkit.NewLakeFSFake(t)
	id := kernel.RepositoryID("kr://same/logical-name")
	one, err := snapshot.OpenLakeFS(id, fake.DSN(fake.NewRepo()), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	two, err := snapshot.OpenLakeFS(id, fake.DSN(fake.NewRepo()), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	if one.(snapshot.StoreOrigin).Origin() == two.(snapshot.StoreOrigin).Origin() {
		t.Fatal("same-endpoint physical repositories share an authority binding")
	}
}
