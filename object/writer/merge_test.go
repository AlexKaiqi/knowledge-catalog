package writer_test

import (
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/object/writer"
	"kc/snapshot"
)

const reviewRef = "refs/heads/review/policy-a"

func strictPolicyAspect(value map[string]any) objectmodel.Operation {
	return objectmodel.Operation{
		Op:        objectmodel.OpPut,
		Address:   objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
		SchemaRef: "schema/policy/structure/v1", Value: value,
	}
}

// mergeFixture publishes a strict Schema with one instance on main, opens a
// review branch from it and writes one valid revision onto the branch.
func mergeFixture(t *testing.T) (testkit.Setup, kernel.CommitID, writer.CommitReceipt) {
	t.Helper()
	s := testkit.NewSetup(t, "")
	published, err := s.Writer.Commit("publish", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: strictPolicySchema()},
			strictPolicyAspect(map[string]any{"name": "freeze", "priority": 1}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	main := published.Result.NewCommit
	branch, err := s.Writer.CreateBranch(writer.BranchIntent{TargetRepository: s.RepositoryID, Ref: reviewRef})
	if err != nil {
		t.Fatal(err)
	}
	if branch.CommitID != main {
		t.Fatalf("branch started at %s, want main %s", branch.CommitID, main)
	}
	revised, err := s.Writer.CommitIntent("revise", writer.CommitIntent{
		TargetRepository: s.RepositoryID, TargetRef: reviewRef,
		Operations: []objectmodel.Operation{strictPolicyAspect(map[string]any{"name": "thaw", "priority": 2})},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != main {
		t.Fatalf("branch write moved main: %s -> %s", main, got)
	}
	return s, main, revised
}

func TestBranchMergePublishesReviewedCommit(t *testing.T) {
	s, main, revised := mergeFixture(t)
	advanced := []snapshot.Advanced{}
	s.Store.OnAdvanced(func(ev snapshot.Advanced) { advanced = append(advanced, ev) })
	intent := writer.MergeIntent{
		TargetRepository: s.RepositoryID, SourceRef: reviewRef,
		SourceCommit: revised.Result.NewCommit, ExpectedTargetCommit: main,
	}
	merged, err := s.Writer.Merge("merge-1", intent)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Disposition != writer.DispositionApplied || merged.Surface != "MERGE" ||
		merged.Result.OldCommit != main || merged.Result.NewCommit != revised.Result.NewCommit {
		t.Fatalf("merge receipt %#v", merged)
	}
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != revised.Result.NewCommit {
		t.Fatalf("main at %s, want reviewed %s", got, revised.Result.NewCommit)
	}
	if len(advanced) != 1 || advanced[0].From != main || advanced[0].To != revised.Result.NewCommit {
		t.Fatalf("published Ref advance not notified exactly once: %#v", advanced)
	}
	value, err := s.Reader.ReadAddress(s.RepositoryID, strictPolicyAspect(nil).Address, testkit.MustHead(t, s.Repo, snapshot.DefaultRef))
	if err != nil || kernel.CanonicalDigest(value.Value) != kernel.CanonicalDigest(map[string]any{"name": "thaw", "priority": 2}) {
		t.Fatalf("main does not serve the reviewed revision: %#v, %v", value, err)
	}
	again, err := s.Writer.Merge("merge-1", writer.MergeIntent{TargetRepository: s.RepositoryID, SourceRef: reviewRef})
	if err != nil || again.Disposition != writer.DispositionReplayed || again.Result.NewCommit != merged.Result.NewCommit {
		t.Fatalf("merge replay %#v, %v", again, err)
	}
	if len(advanced) != 1 {
		t.Fatalf("replay notified again: %#v", advanced)
	}
}

func TestBranchWriteDoesNotNotifyPublishedSubscribers(t *testing.T) {
	s, main, _ := mergeFixture(t)
	advanced := 0
	s.Store.OnAdvanced(func(snapshot.Advanced) { advanced++ })
	if _, err := s.Writer.CommitIntent("revise-again", writer.CommitIntent{
		TargetRepository: s.RepositoryID, TargetRef: reviewRef,
		Operations: []objectmodel.Operation{strictPolicyAspect(map[string]any{"name": "again", "priority": 3})},
	}); err != nil {
		t.Fatal(err)
	}
	if advanced != 0 || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != main {
		t.Fatalf("working branch write reached published subscribers: %d", advanced)
	}
}

func TestMergeRejectsSourceMovedAfterReview(t *testing.T) {
	s, main, revised := mergeFixture(t)
	if _, err := s.Writer.CommitIntent("late", writer.CommitIntent{
		TargetRepository: s.RepositoryID, TargetRef: reviewRef,
		Operations: []objectmodel.Operation{strictPolicyAspect(map[string]any{"name": "late", "priority": 9})},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Writer.Merge("merge-moved", writer.MergeIntent{
		TargetRepository: s.RepositoryID, SourceRef: reviewRef,
		SourceCommit: revised.Result.NewCommit, ExpectedTargetCommit: main,
	})
	testkit.ExpectCode(t, err, kernel.ErrCandidateMoved)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != main {
		t.Fatalf("rejected merge moved main to %s", got)
	}
	if _, ok := s.Writer.Lookup("merge-moved"); ok {
		t.Fatal("rejected merge left an accepted receipt")
	}
}

func TestMergeRejectsTargetDrift(t *testing.T) {
	s, main, revised := mergeFixture(t)
	drift, err := s.Writer.Commit("drift", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: main, ExpectedTargetCommit: main,
		Operations: testkit.PutEntity("fixture/other", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Writer.Merge("merge-stale", writer.MergeIntent{
		TargetRepository: s.RepositoryID, SourceRef: reviewRef,
		SourceCommit: revised.Result.NewCommit, ExpectedTargetCommit: main,
	})
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
	// A fresh CAS still cannot publish a branch that no longer descends from main.
	_, err = s.Writer.Merge("merge-diverged", writer.MergeIntent{TargetRepository: s.RepositoryID, SourceRef: reviewRef})
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != drift.Result.NewCommit {
		t.Fatalf("rejected merge moved main to %s", got)
	}
}

// rawBranchWrite bypasses Writer to put literal bytes on the review branch,
// standing in for any producer that edits the branch without the contract.
func rawBranchWrite(t *testing.T, s testkit.Setup, changes []snapshot.TreeChange) kernel.CommitID {
	t.Helper()
	head := testkit.MustHead(t, s.Repo, reviewRef)
	commit, err := s.Repo.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: reviewRef,
		BaseCommit: head, ExpectedTargetCommit: head, Changes: changes,
	})
	if err != nil {
		t.Fatal(err)
	}
	return commit
}

func aspectPath(t *testing.T, s testkit.Setup, commit kernel.CommitID) string {
	t.Helper()
	paths, err := repofile.ReadObjectLocator(s.Repo, "policy/A", commit)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.Contains(path, "structure") {
			return path
		}
	}
	t.Fatalf("no structure unit in %v", paths)
	return ""
}

func TestMergeValidatesSourceObjectsAgainstSchema(t *testing.T) {
	s, main, revised := mergeFixture(t)
	path := aspectPath(t, s, revised.Result.NewCommit)
	address := strictPolicyAspect(nil).Address
	body, err := repofile.Serialize(address, path, "schema/policy/structure/v1", nil, map[string]any{"name": "raw", "priority": "high"})
	if err != nil {
		t.Fatal(err)
	}
	source := rawBranchWrite(t, s, []snapshot.TreeChange{{Path: path, Content: []byte(body)}})
	_, err = s.Writer.Merge("merge-invalid", writer.MergeIntent{
		TargetRepository: s.RepositoryID, SourceRef: reviewRef, SourceCommit: source, ExpectedTargetCommit: main,
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != main {
		t.Fatalf("invalid source published: main at %s", got)
	}
}

func TestMergeRejectsUnlocatedAndNonObjectSourceFiles(t *testing.T) {
	for name, change := range map[string]struct {
		changes func(path string) []snapshot.TreeChange
		code    kernel.ErrorCode
	}{
		"non-object file": {
			changes: func(string) []snapshot.TreeChange {
				return []snapshot.TreeChange{{Path: "build/output.bin", Content: []byte{0, 1}}}
			},
			code: kernel.ErrUsageInvalid,
		},
		"unit removed behind locator": {
			changes: func(path string) []snapshot.TreeChange {
				return []snapshot.TreeChange{{Path: path, Remove: true}}
			},
			code: kernel.ErrPreconditionFailed,
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, main, revised := mergeFixture(t)
			source := rawBranchWrite(t, s, change.changes(aspectPath(t, s, revised.Result.NewCommit)))
			_, err := s.Writer.Merge("merge-"+strings.ReplaceAll(name, " ", "-"), writer.MergeIntent{
				TargetRepository: s.RepositoryID, SourceRef: reviewRef, SourceCommit: source, ExpectedTargetCommit: main,
			})
			testkit.ExpectCode(t, err, change.code)
			if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != main {
				t.Fatalf("rejected source published: main at %s", got)
			}
		})
	}
}

func TestBranchRequiresNewRefAndKnownBasis(t *testing.T) {
	s, _, _ := mergeFixture(t)
	_, err := s.Writer.CreateBranch(writer.BranchIntent{TargetRepository: s.RepositoryID, Ref: reviewRef})
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
	_, err = s.Writer.CreateBranch(writer.BranchIntent{TargetRepository: s.RepositoryID, Ref: "refs/heads/other", FromCommit: "missing"})
	testkit.ExpectCode(t, err, kernel.ErrVersionUnresolved)
	_, err = s.Writer.Merge("self", writer.MergeIntent{TargetRepository: s.RepositoryID, SourceRef: snapshot.DefaultRef})
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

func TestSystemRepositoryRefusesBranchAndMerge(t *testing.T) {
	s := testkit.NewSetup(t, string(objectmodel.SystemRepositoryID))
	_, err := s.Writer.CreateBranch(writer.BranchIntent{TargetRepository: s.RepositoryID, Ref: reviewRef})
	testkit.ExpectCode(t, err, kernel.ErrForbidden)
	_, err = s.Writer.Merge("system", writer.MergeIntent{TargetRepository: s.RepositoryID, SourceRef: reviewRef})
	testkit.ExpectCode(t, err, kernel.ErrForbidden)
}
