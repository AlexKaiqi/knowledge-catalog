package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	kcclient "kc/client"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
)

type desiredReplayRemote struct {
	setup    testkit.Setup
	commands snapshot.CommandLedger
	store    snapshot.CommandStore
	posts    atomic.Int32
	server   *httptest.Server
}

func newDesiredReplayRemote(t *testing.T) *desiredReplayRemote {
	t.Helper()
	isolateLoginConfig(t)
	f := &desiredReplayRemote{setup: testkit.NewSetup(t, initialUploadRepository)}
	var err error
	f.store = snapshot.NewBoltCommandStore(filepath.Join(t.TempDir(), "commands.db"))
	f.commands, err = snapshot.NewCommandLedger(f.store)
	if err != nil {
		t.Fatal(err)
	}
	f.setup.Writer, err = writer.NewWriter(f.setup.Store, f.commands)
	if err != nil {
		t.Fatal(err)
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.server.Close)
	return f
}

func (f *desiredReplayRemote) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var result any
	var err error
	switch {
	case strings.HasSuffix(r.URL.Path, "/head"):
		var commit kernel.CommitID
		commit, err = f.setup.Repo.Head(snapshot.RefOrDefault(r.URL.Query().Get("ref")))
		result = map[string]any{"commit": commit}
	case r.URL.Path == "/object/v1/objects:resolve":
		var request kcclient.ObjectResolveRequest
		if err = json.NewDecoder(r.Body).Decode(&request); err == nil {
			result, err = f.setup.Reader.ResolveAddress(kernel.RepositoryID(request.Repository),
				objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(request.Object)}, kernel.CommitID(request.Commit))
		}
	case strings.HasSuffix(r.URL.Path, "/commits"):
		f.posts.Add(1)
		var request kcclient.CommitRequest
		if err = json.NewDecoder(r.Body).Decode(&request); err == nil {
			cs := request.ChangeSet
			result, err = f.setup.Writer.CommitIntent(request.CommandID, writer.CommitIntent{
				TargetRepository: cs.TargetRepository, TargetRef: cs.TargetRef,
				BaseCommit: cs.BaseCommit, ExpectedTargetCommit: cs.ExpectedTargetCommit,
				Operations: cs.Operations, Message: cs.Message, Provenance: cs.Provenance, BulkIngest: cs.BulkIngest,
			})
		}
	case strings.Contains(r.URL.Path, "/receipts/"):
		var ok bool
		result, ok = f.commands.Lookup(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
		if !ok {
			err = kernel.Fail(kernel.ErrUsageInvalid, "unknown command-id")
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		result = kernel.FaultJSON(err)
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (f *desiredReplayRemote) seed(t *testing.T, command, id, body string) {
	t.Helper()
	base := testkit.MustHead(t, f.setup.Repo, snapshot.DefaultRef)
	_, err := f.setup.Writer.Commit(command, objectmodel.CommitChangeSet{
		TargetRepository: f.setup.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(id)},
			Value:   map[string]any{"body": body}}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRemoteDesiredReplayRetainsOriginalFullSelection(t *testing.T) {
	f := newDesiredReplayRemote(t)
	f.seed(t, "existing", "note/item-0", "item-0")
	dir := initialUploadDirectory(t, 2)
	first := runInitialUpload(f.server, dir, "incremental-retry", "--bulk")
	second := runInitialUpload(f.server, dir, "incremental-retry", "--bulk")
	if first.Status != 0 || second.Status != 0 {
		t.Fatalf("incremental retry failed: first=%s second=%s", first.Stdout, second.Stdout)
	}
	entry, ok := f.commands.Lookup("incremental-retry")
	if !ok || entry.Request.OperationCount != 2 || f.posts.Load() != 2 || !strings.Contains(second.Stdout, `"REPLAYED"`) {
		t.Fatalf("retry did not verify the original full request: operations=%d posts=%d second=%s", entry.Request.OperationCount, f.posts.Load(), second.Stdout)
	}
}

func TestRemoteDesiredReplayRejectsIncompleteOrPendingEvidence(t *testing.T) {
	for name, damage := range map[string]func(*snapshot.CommandEntry){
		"missing request":    func(e *snapshot.CommandEntry) { e.Request = snapshot.CommandRequest{} },
		"pending":            func(e *snapshot.CommandEntry) { e.Status = snapshot.StatusPending },
		"missing kind":       func(e *snapshot.CommandEntry) { e.Request.Kind = "" },
		"missing repository": func(e *snapshot.CommandEntry) { e.Request.RepositoryID = "" },
		"missing ref":        func(e *snapshot.CommandEntry) { e.Request.TargetRef = "" },
	} {
		t.Run(name, func(t *testing.T) {
			f := newDesiredReplayRemote(t)
			dir := initialUploadDirectory(t, 1)
			first := runInitialUpload(f.server, dir, "unverifiable")
			if first.Status != 0 {
				t.Fatal(first.Stdout)
			}
			entry, _ := f.commands.Lookup("unverifiable")
			damage(&entry)
			if err := f.store.Put(entry); err != nil {
				t.Fatal(err)
			}
			before := testkit.MustHead(t, f.setup.Repo, snapshot.DefaultRef)
			result := runInitialUpload(f.server, dir, "unverifiable")
			code := kernel.ErrPreconditionFailed
			if result.Status == 0 || !strings.Contains(result.Stdout, string(code)) {
				t.Fatalf("incomplete or uncertain receipt replayed: %#v", result)
			}
			if after := testkit.MustHead(t, f.setup.Repo, snapshot.DefaultRef); after != before {
				t.Fatal("unverifiable retry advanced authority")
			}

		})
	}
}

// Receipt recovery and historical previews retain their own read failures;
// publishing a full intent no longer performs either read as a preflight.
func TestRemoteDesiredReadsDoNotHideReceiptOrHistoricalReadFailure(t *testing.T) {
	for _, failure := range []string{"receipt permission", "receipt backend", "historical permission"} {
		t.Run(failure, func(t *testing.T) {
			f, server := newInitialUploadRemote(t, `{"commit":"current","empty":false}`)
			remote, err := kcclient.New(kcclient.Config{BaseURL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = remote.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: "agent:supplier"}}); err != nil {
				t.Fatal(err)
			}
			code := kernel.ErrForbidden
			if failure == "historical permission" {
				f.resolveErrorAt = "old"
				_, err = remoteCurrentDigests(context.Background(), remote, initialUploadRepository, "old", []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/item-0"}}}, kcclient.RequestOptions{})
			} else {
				f.receiptError = kernel.ErrForbidden
				if failure == "receipt backend" {
					f.receiptError = kernel.ErrTemporaryUnavailable
					code = kernel.ErrTemporaryUnavailable
				}
				var entry snapshot.CommandEntry
				err = remote.WriterService().Receipt(context.Background(), "blocked", kcclient.RequestOptions{}, &entry)
			}
			testkit.ExpectCode(t, err, code)
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.commits) != 0 {
				t.Fatal("failed read submitted a mutation")
			}
		})
	}
}

func TestRemoteDesiredReplayRejectsChangedIntentDespiteMatchingCurrentValues(t *testing.T) {
	for _, change := range []string{"bulk", "content", "ref", "base", "provenance", "schema", "path"} {
		t.Run(change, func(t *testing.T) {
			f := newDesiredReplayRemote(t)
			dir := initialUploadDirectory(t, 1)
			first := runInitialUpload(f.server, dir, "intent-retry", "--bulk")
			if first.Status != 0 {
				t.Fatal(first.Stdout)
			}
			before := testkit.MustHead(t, f.setup.Repo, snapshot.DefaultRef)
			extra := []string{"--bulk"}
			switch change {
			case "bulk":
				extra = nil
			case "content":
				f.seed(t, "other-publisher", "note/item-0", "changed")
				before = testkit.MustHead(t, f.setup.Repo, snapshot.DefaultRef)
				data := "---\nobject_id: note/item-0\n---\n{\"body\":\"changed\"}\n"
				if err := os.WriteFile(filepath.Join(dir, "item-0.json"), []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			case "ref":
				if err := f.setup.Repo.CreateRef("refs/heads/review", before); err != nil {
					t.Fatal(err)
				}
				extra = append(extra, "--ref", "refs/heads/review")
			case "base":
				extra = append(extra, "--base", string(before))
			case "provenance":
				extra = append(extra, "--origin-kind", "SOURCE", "--source-ref", "different-source")
			case "schema", "path":
				header := "schema_ref: schema/another\n"
				if change == "path" {
					header = "path_hint: relocated/one.json\n"
				}
				data := "---\nobject_id: note/item-0\n" + header + "---\n{\"body\":\"item-0\"}\n"
				if err := os.WriteFile(filepath.Join(dir, "item-0.json"), []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			second := runInitialUpload(f.server, dir, "intent-retry", extra...)
			if second.Status == 0 || !strings.Contains(second.Stdout, string(kernel.ErrIdempotencyConflict)) {
				t.Fatalf("changed %s was accepted as replay: %#v", change, second)
			}
			if got := testkit.MustHead(t, f.setup.Repo, snapshot.DefaultRef); got != before {
				t.Fatalf("rejected replay changed target: %s -> %s", before, got)
			}
		})
	}
}
