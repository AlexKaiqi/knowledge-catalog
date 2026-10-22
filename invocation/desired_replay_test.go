package invocation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"kc/home"
	"kc/internal/testkit"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
)

func desiredReplayInvocation(t *testing.T) (*Invocation, testkit.Setup) {
	t.Helper()
	s := testkit.NewSetup(t, "kr://acme/desired-retry")
	commands, err := snapshot.NewCommandLedger(nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Writer, err = writer.NewWriter(s.Store, commands)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for i := 0; i < 2; i++ {
		data := fmt.Sprintf("---\nobject_id: note/item-%d\n---\n{\"body\":\"item-%d\"}\n", i, i)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("item-%d.json", i)), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return &Invocation{Context: context.Background(),
		WS:    &home.Home{Store: s.Store, Reader: s.Reader, Writer: s.Writer, Commands: commands},
		Flags: map[string]invoke.FlagValue{"repo": string(s.RepositoryID), "dir": dir, "command-id": "directory", "bulk": true}}, s
}

func TestEmbeddedDesiredReplayChecksChangedIntent(t *testing.T) {
	for _, change := range []string{"bulk", "content", "ref", "base"} {
		t.Run(change, func(t *testing.T) {
			cx, s := desiredReplayInvocation(t)
			if _, err := CommitDesiredDir(cx); err != nil {
				t.Fatal(err)
			}
			before := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
			switch change {
			case "bulk":
				cx.Flags["bulk"] = false
			case "content":
				_, err := s.Writer.Commit("other-publisher", objectmodel.CommitChangeSet{
					TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
					BaseCommit: before, ExpectedTargetCommit: before,
					Operations: []objectmodel.Operation{{Op: objectmodel.OpPut,
						Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/item-0"},
						Value:   map[string]any{"body": "changed"}}},
				})
				if err != nil {
					t.Fatal(err)
				}
				before = testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
				if err := os.WriteFile(filepath.Join(cx.Flag("dir"), "item-0.json"), []byte("---\nobject_id: note/item-0\n---\n{\"body\":\"changed\"}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "ref":
				if err := s.Repo.CreateRef("refs/heads/review", before); err != nil {
					t.Fatal(err)
				}
				cx.Flags["ref"] = "refs/heads/review"
			case "base":
				cx.Flags["base"] = string(before)
			}
			_, err := CommitDesiredDir(cx)
			testkit.ExpectCode(t, err, kernel.ErrIdempotencyConflict)
			if after := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); after != before {
				t.Fatalf("rejected retry advanced target: %s -> %s", before, after)
			}
		})
	}
}

func TestEmbeddedDesiredReplayRetainsOriginalFullRequest(t *testing.T) {
	cx, s := desiredReplayInvocation(t)
	_, err := s.Writer.Commit("existing", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/item-0"},
			Value:   map[string]any{"body": "item-0"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := CommitDesiredDir(cx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CommitDesiredDir(cx)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := cx.WS.Commands.Lookup("directory")
	one := first.(writer.CommitReceipt)
	two := second.(writer.CommitReceipt)
	if !ok || entry.Request.OperationCount != 2 || two.Disposition != writer.DispositionReplayed || one.Result.NewCommit != two.Result.NewCommit {
		t.Fatalf("retry changed the original incremental request: operations=%d first=%#v second=%#v", entry.Request.OperationCount, one, two)
	}
	cx.Flags["command-id"] = "new-noop"
	_, err = CommitDesiredDir(cx)
	if err != nil {
		t.Fatal(err)
	}
}

type desiredReplayEntryLedger struct {
	snapshot.CommandLedger
	entry snapshot.CommandEntry
}

func (l desiredReplayEntryLedger) Lookup(string) (snapshot.CommandEntry, bool) { return l.entry, true }

func TestEmbeddedDesiredReplayRejectsIncompleteTargetMetadata(t *testing.T) {
	for name, damage := range map[string]func(*snapshot.CommandRequest){
		"kind":       func(r *snapshot.CommandRequest) { r.Kind = "" },
		"repository": func(r *snapshot.CommandRequest) { r.RepositoryID = "" },
		"ref":        func(r *snapshot.CommandRequest) { r.TargetRef = "" },
	} {
		t.Run(name, func(t *testing.T) {
			cx, s := desiredReplayInvocation(t)
			if _, err := CommitDesiredDir(cx); err != nil {
				t.Fatal(err)
			}
			entry, _ := cx.WS.Commands.Lookup(cx.Flag("command-id"))
			damage(&entry.Request)
			cx.WS.Commands = desiredReplayEntryLedger{CommandLedger: cx.WS.Commands, entry: entry}
			var err error
			cx.WS.Writer, err = writer.NewWriter(s.Store, cx.WS.Commands)
			if err != nil {
				t.Fatal(err)
			}
			before := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
			_, err = CommitDesiredDir(cx)
			testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
			if after := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); after != before {
				t.Fatalf("incomplete replay metadata advanced target: %s -> %s", before, after)
			}
		})
	}
}
