package service_test

import (
	"context"
	"fmt"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/service"
	"kc/object/writer"
	"kc/snapshot"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBoundedHistoryServicePersistsEmptyPagesAcrossReopenAndSourceAdvance(t *testing.T) {
	fake := testkit.NewLakeFSFake(t)
	source, err := snapshot.OpenLakeFS("kr://history/service", fake.DSN(fake.NewRepo()), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	registry := snapshot.NewRegistry()
	if err := registry.Add(source); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	at, err := source.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	sequence := 0
	put := func(id objectmodel.ObjectID, value string) kernel.CommitID {
		t.Helper()
		sequence++
		receipt, err := w.Commit(fmt.Sprintf("history-service-%d", sequence), objectmodel.ChangeSet{TargetRepository: source.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: id}, Value: value}}})
		if err != nil {
			t.Fatal(err)
		}
		at = receipt.Result.NewCommit
		return at
	}
	first := put("note/history", "A")
	for i := 0; i < 12; i++ {
		put("note/unrelated", fmt.Sprint(i))
	}
	second := put("note/history", "B")
	third := put("note/history", "A")
	repo, err := reader.NewReader(registry).Require(source.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	basis := objectmodel.ReadBasis{Version: 1, Reference: "urn:kc:object:note/history", Candidates: []objectmodel.ReadSource{{Repository: source.ID(), Commit: at, Binding: repo.(objectmodel.StoreIdentified).StoreDigest()}}, Paths: []objectmodel.ReadPath{{Repository: source.ID(), Kind: "prefix"}}}
	path := filepath.Join(t.TempDir(), "history.db")
	request := service.ObjectLogRequest{Object: "note/history", Basis: basis, Ref: snapshot.DefaultRef, Limit: 1, Budget: 2}
	var revisions []kernel.CommitID
	empty := 0
	complete := false
	for pageNumber := 0; pageNumber < 32; pageNumber++ {
		records, err := reader.NewHistoryCheckpointStore(path)
		if err != nil {
			t.Fatal(err)
		}
		rd := reader.NewReader(registry)
		executor := service.ObjectLogExecutor{Checkpoints: records, ReadScan: func(ctx context.Context, id kernel.RepositoryID, commit kernel.CommitID, query objectmodel.HistoryScanQuery) (objectmodel.HistoryScanPage, error) {
			return rd.ScanHistory(ctx, id, request.Object, commit, query)
		}}
		page, err := executor.Execute(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if page.Scanned == nil || *page.Scanned > 2 {
			t.Fatalf("global budget exceeded: %#v", page)
		}
		for _, log := range page.Logs {
			if log.Commit != basis.Candidates[0].Commit {
				t.Fatal("page adopted a new source head")
			}
			for _, revision := range log.Revisions {
				revisions = append(revisions, revision.Commit)
			}
		}
		if page.Complete != nil && *page.Complete {
			if page.Continuation != "" {
				t.Fatal("finished page retained a continuation")
			}
			complete = true
			break
		}
		if len(page.Logs) == 0 {
			empty++
		}
		if page.Continuation == "" || page.Continuation == request.Continuation {
			t.Fatal("unfinished page did not advance")
		}
		if pageNumber == 0 {
			put("note/history", "newer-than-request")
		}
		request.Continuation = page.Continuation
	}
	if !complete || empty < 4 || !reflect.DeepEqual(revisions, []kernel.CommitID{third, second, first}) {
		t.Fatalf("lost original history: complete=%v empty=%d revisions=%v", complete, empty, revisions)
	}
}

func TestBoundedHistoryServiceRejectsScopeChangeAndRevocationBeforeSourceReads(t *testing.T) {
	basis := objectmodel.ReadBasis{Version: 1, Reference: "urn:kc:object:note/history", Candidates: []objectmodel.ReadSource{{Repository: "kr://history/a", Commit: "a1", Binding: "authority-a"}, {Repository: "kr://history/b", Commit: "b1", Binding: "authority-b"}}, Paths: []objectmodel.ReadPath{{Repository: "kr://history/a", Kind: "prefix"}, {Repository: "kr://history/b", Kind: "prefix"}}}
	records, err := reader.NewHistoryCheckpointStore(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := objectmodel.HistoryCheckpoint{Version: 1, Basis: basis, Ref: snapshot.DefaultRef, Open: map[kernel.RepositoryID]*objectmodel.HistoryScanProgress{"kr://history/b": nil}}
	reference, err := records.Retain(context.Background(), checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	executor := service.ObjectLogExecutor{Checkpoints: records, ReadScan: func(context.Context, kernel.RepositoryID, kernel.CommitID, objectmodel.HistoryScanQuery) (objectmodel.HistoryScanPage, error) {
		calls++
		return objectmodel.HistoryScanPage{Complete: true}, nil
	}}
	request := service.ObjectLogRequest{Object: "note/history", Basis: basis, Ref: snapshot.DefaultRef, Limit: 1, Continuation: reference}
	executor.Allow = func(id kernel.RepositoryID, _ objectmodel.ObjectID) bool { return id != "kr://history/a" }
	if _, err := executor.Execute(context.Background(), request); kernel.CodeOf(err) != kernel.ErrForbidden || calls != 0 {
		t.Fatalf("completed source escaped current authorization: %v calls=%d", err, calls)
	}
	executor.Allow = nil
	for _, change := range []string{"ref", "commit", "object", "binding"} {
		altered := request
		altered.Basis.Candidates = append([]objectmodel.ReadSource(nil), basis.Candidates...)
		switch change {
		case "ref":
			altered.Ref = "refs/heads/other"
		case "commit":
			altered.Basis.Candidates[0].Commit = "newer"
		case "object":
			altered.Object = "note/other"
		case "binding":
			altered.Basis.Candidates[0].Binding = "replaced"
		}
		if _, err := executor.Execute(context.Background(), altered); kernel.CodeOf(err) != kernel.ErrUsageInvalid || calls != 0 {
			t.Fatalf("changed %s read source: %v calls=%d", change, err, calls)
		}
	}
}

func TestLegacyRevisionOnlyHistoryDoesNotInventScanMeasurement(t *testing.T) {
	executor := service.ObjectLogExecutor{ReadLog: func(kernel.RepositoryID, kernel.CommitID, objectmodel.ObjectLogQuery) ([]objectmodel.ObjectRevision, error) {
		return []objectmodel.ObjectRevision{{Commit: "c1", Status: objectmodel.StatusResolved}}, nil
	}}
	page, err := executor.Execute(context.Background(), service.ObjectLogRequest{Object: "note/history", Pin: map[kernel.RepositoryID]kernel.CommitID{"kr://history/source": "c1"}, Limit: 2})
	if err != nil || len(page.Logs) != 1 || page.Scanned != nil || page.Complete != nil {
		t.Fatalf("revision-only provider fabricated bounded scan evidence: %#v %v", page, err)
	}
}

type failingHistoryRetention struct {
	reader.HistoryCheckpointStore
	fail bool
}

func (s *failingHistoryRetention) Retain(ctx context.Context, checkpoint objectmodel.HistoryCheckpoint) (string, error) {
	if s.fail {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "injected checkpoint persistence failure")
	}
	return s.HistoryCheckpointStore.Retain(ctx, checkpoint)
}

func TestBoundedHistoryRejectsBadProviderProgressAndNeverReturnsUnsavedSuccess(t *testing.T) {
	source := objectmodel.ReadSource{Repository: "kr://history/source", Commit: "c1", Binding: "source-authority"}
	basis := objectmodel.ReadBasis{Version: 1, Reference: "urn:kc:object:note/history", Candidates: []objectmodel.ReadSource{source}, Paths: []objectmodel.ReadPath{{Repository: source.Repository, Kind: "prefix"}}}
	progress := objectmodel.HistoryScanProgress{Algorithm: objectmodel.LocalStateAlgorithm, Object: "note/history", Repository: source.Repository, Binding: source.Binding, Start: source.Commit, Position: "p0", Pending: objectmodel.ObjectRevision{Commit: "p0", Status: objectmodel.StatusResolved}, State: "state"}
	records, err := reader.NewHistoryCheckpointStore(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	original, err := records.Retain(context.Background(), objectmodel.HistoryCheckpoint{Version: 1, Basis: basis, Ref: snapshot.DefaultRef, Open: map[kernel.RepositoryID]*objectmodel.HistoryScanProgress{source.Repository: &progress}})
	if err != nil {
		t.Fatal(err)
	}
	request := service.ObjectLogRequest{Object: "note/history", Basis: basis, Ref: snapshot.DefaultRef, Limit: 3, Budget: 3, Continuation: original}
	for _, fault := range []string{"budget", "stalled", "repeated-position", "scope", "algorithm", "too-many-revisions", "complete-with-progress"} {
		t.Run(fault, func(t *testing.T) {
			next := progress
			next.Position = "p1"
			next.Pending.Commit = "p1"
			page := objectmodel.HistoryScanPage{Scanned: 1, Progress: &next}
			switch fault {
			case "budget":
				page.Scanned = 4
			case "stalled":
				page.Scanned = 0
			case "repeated-position":
				next.Position = "p0"
				next.Pending.Commit = "p0"
			case "scope":
				next.Binding = "rebound"
			case "algorithm":
				next.Algorithm = "different"
			case "too-many-revisions":
				page.Complete = true
				page.Progress = nil
				page.Revisions = []objectmodel.ObjectRevision{{Commit: "one"}, {Commit: "two"}, {Commit: "three"}}
			case "complete-with-progress":
				page.Complete = true
			}
			executor := service.ObjectLogExecutor{Checkpoints: records, ReadScan: func(context.Context, kernel.RepositoryID, kernel.CommitID, objectmodel.HistoryScanQuery) (objectmodel.HistoryScanPage, error) {
				return page, nil
			}}
			result, err := executor.Execute(context.Background(), request)
			if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || result.Continuation != "" || len(result.Logs) != 0 {
				t.Fatalf("bad provider %s returned a successful page: %#v %v", fault, result, err)
			}
		})
	}
	retention := &failingHistoryRetention{HistoryCheckpointStore: records, fail: true}
	executor := service.ObjectLogExecutor{Checkpoints: retention, ReadScan: func(_ context.Context, _ kernel.RepositoryID, _ kernel.CommitID, query objectmodel.HistoryScanQuery) (objectmodel.HistoryScanPage, error) {
		query.Progress.Position = "p1"
		query.Progress.Pending.Commit = "p1"
		return objectmodel.HistoryScanPage{Scanned: 1, Progress: query.Progress}, nil
	}}
	if page, err := executor.Execute(context.Background(), request); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || page.Continuation != "" || page.Complete != nil {
		t.Fatalf("unsaved progress returned success: %#v %v", page, err)
	}
	retention.fail = false
	page, err := executor.Execute(context.Background(), request)
	if err != nil || page.Continuation == "" || page.Continuation == original || page.Complete == nil || *page.Complete {
		t.Fatalf("provider-owned input mutation corrupted progress check: %#v %v", page, err)
	}
	old, err := records.Resolve(context.Background(), original)
	if err != nil || old.Open[source.Repository].Position != "p0" {
		t.Fatalf("failed or advanced page overwrote original progress: %#v %v", old, err)
	}
}
