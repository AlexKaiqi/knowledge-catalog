package reader_test

import (
	"context"
	"errors"
	"fmt"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
	"reflect"
	"testing"
	"time"
)

func TestHistoryScanKeepsUnprovenSegmentAcrossEmptyPages(t *testing.T) {
	_, guard, w, base := newReferenceReadFixture(t)
	at := base
	command := 0
	put := func(id string, value string) kernel.CommitID {
		t.Helper()
		command++
		receipt, err := w.Commit(fmt.Sprintf("scan-%d", command), objectmodel.ChangeSet{TargetRepository: guard.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(id)}, Value: value}}})
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
	history, ok := snapshot.HistoryStoreOf(guard.Store)
	if !ok {
		t.Fatal("fixture has no actual commit history")
	}
	registry := snapshot.NewRegistry()
	if err := registry.Add(&comparisonHistoryStore{referenceReadGuard: guard, HistoryStore: history}); err != nil {
		t.Fatal(err)
	}
	open := func() objectmodel.HistoryScanner {
		repo, err := reader.NewReader(registry).Require(guard.ID(), kernel.ErrObjectRefUnresolved)
		if err != nil {
			t.Fatal(err)
		}
		scanner, ok := repo.(objectmodel.HistoryScanner)
		if !ok {
			t.Fatal("tree repository has no bounded scanner")
		}
		return scanner
	}
	var progress *objectmodel.HistoryScanProgress
	var commits []kernel.CommitID
	empty := 0
	complete := false
	for pageNumber := 0; pageNumber < 32; pageNumber++ {
		guard.commits = nil
		page, err := open().ScanHistory(context.Background(), "note/history", at, objectmodel.HistoryScanQuery{Limit: 1, Budget: 2, Progress: progress})
		if err != nil {
			t.Fatal(err)
		}
		reads := map[kernel.CommitID]bool{}
		for _, commit := range guard.commits {
			reads[commit] = true
		}
		if page.Scanned > 2 || len(reads) > 2 {
			t.Fatalf("scan budget did not bound source reads: scanned=%d commits=%v", page.Scanned, reads)
		}
		if len(page.Revisions) == 0 && !page.Complete {
			empty++
		}
		for _, revision := range page.Revisions {
			commits = append(commits, revision.Commit)
		}
		if page.Complete {
			complete = true
			break
		}
		if page.Progress == nil || page.Scanned == 0 || progress != nil && page.Progress.Position == progress.Position {
			t.Fatalf("unfinished page made no progress: %#v", page)
		}
		// Recreate Reader each time; the unfinished segment is fully represented.
		progress = page.Progress
	}
	if !complete || empty < 4 || !reflect.DeepEqual(commits, []kernel.CommitID{third, second, first}) {
		t.Fatalf("fabricated or lost introduction: complete=%v empty=%d commits=%v", complete, empty, commits)
	}
}

func TestHistoryScanRejectsChangedCoordinatesAndCancellationBeforeReads(t *testing.T) {
	_, guard, _, at := newReferenceReadFixture(t)
	history, ok := snapshot.HistoryStoreOf(guard.Store)
	if !ok {
		t.Fatal("history fixture missing")
	}
	registry := snapshot.NewRegistry()
	if err := registry.Add(&comparisonHistoryStore{referenceReadGuard: guard, HistoryStore: history}); err != nil {
		t.Fatal(err)
	}
	rd := reader.NewReader(registry)
	first, err := rd.ScanHistory(context.Background(), guard.ID(), "table/orders", at, objectmodel.HistoryScanQuery{Limit: 1, Budget: 1})
	if err != nil || first.Progress == nil {
		t.Fatalf("missing unfinished segment: %#v %v", first, err)
	}
	for name, damage := range map[string]func(*objectmodel.HistoryScanProgress){
		"algorithm":  func(p *objectmodel.HistoryScanProgress) { p.Algorithm = "unsupported" },
		"object":     func(p *objectmodel.HistoryScanProgress) { p.Object = "table/other" },
		"repository": func(p *objectmodel.HistoryScanProgress) { p.Repository = "kr://other/source" },
		"binding":    func(p *objectmodel.HistoryScanProgress) { p.Binding = "different" },
		"start":      func(p *objectmodel.HistoryScanProgress) { p.Start = "other" },
		"position":   func(p *objectmodel.HistoryScanProgress) { p.Position = "other" },
		"after":      func(p *objectmodel.HistoryScanProgress) { p.After = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			progress := *first.Progress
			damage(&progress)
			guard.commits = nil
			_, err := rd.ScanHistory(context.Background(), guard.ID(), "table/orders", at, objectmodel.HistoryScanQuery{Limit: 1, Budget: 1, Progress: &progress})
			if kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(guard.commits) != 0 {
				t.Fatalf("changed coordinate read source: %v %v", err, guard.commits)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	guard.commits = nil
	_, err = rd.ScanHistory(ctx, guard.ID(), "table/orders", at, objectmodel.HistoryScanQuery{Limit: 1, Budget: 1})
	if err != context.Canceled || len(guard.commits) != 0 {
		t.Fatalf("canceled scan read source: %v %v", err, guard.commits)
	}
}

func TestHistoryScanDoesNotTurnReadFailureOrCancellationIntoAnEmptyPage(t *testing.T) {
	for _, cancelDuringRead := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelDuringRead), func(t *testing.T) {
			_, guard, _, at := newReferenceReadFixture(t)
			history, _ := snapshot.HistoryStoreOf(guard.Store)
			registry := snapshot.NewRegistry()
			if err := registry.Add(&comparisonHistoryStore{referenceReadGuard: guard, HistoryStore: history}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelDuringRead {
				guard.beforeRead = func(string, kernel.CommitID) { cancel() }
			} else {
				guard.blocked["private/description.yaml"] = true
			}
			page, err := reader.NewReader(registry).ScanHistory(ctx, guard.ID(), "table/orders", at, objectmodel.HistoryScanQuery{Limit: 1, Budget: 1})
			if err == nil || page.Progress != nil || page.Complete || len(page.Revisions) != 0 {
				t.Fatalf("failed read became successful budget exhaustion: %#v %v", page, err)
			}
			if cancelDuringRead && err != context.Canceled {
				t.Fatalf("cancellation lost: %v", err)
			}
		})
	}
}

func TestLegacyLogKeepsTheRepositoryRequestContext(t *testing.T) {
	fake := testkit.NewLakeFSFake(t)
	source, err := snapshot.OpenLakeFS("kr://history/context", fake.DSN(fake.NewRepo()), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	at, err := source.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	registry := snapshot.NewRegistry()
	if err := registry.Add(source); err != nil {
		t.Fatal(err)
	}
	repo, err := reader.NewReader(registry).Require(source.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bound := objectmodel.BindContext(ctx, repo)
	_, err = bound.Log("note/history", at, objectmodel.ObjectLogQuery{Limit: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("legacy log replaced its request context: %v", err)
	}
}

func TestHistoryScanTimeBudgetYieldsOnlyAfterActualProgress(t *testing.T) {
	_, guard, _, at := newReferenceReadFixture(t)
	history, _ := snapshot.HistoryStoreOf(guard.Store)
	registry := snapshot.NewRegistry()
	if err := registry.Add(&comparisonHistoryStore{referenceReadGuard: guard, HistoryStore: history}); err != nil {
		t.Fatal(err)
	}
	rd := reader.NewReader(registry)
	slow := true
	guard.beforeRead = func(string, kernel.CommitID) {
		if slow {
			slow = false
			time.Sleep(100 * time.Millisecond)
		}
	}
	page, err := rd.ScanHistory(context.Background(), guard.ID(), "table/orders", at, objectmodel.HistoryScanQuery{Limit: 10, Budget: 100, TimeBudget: 50 * time.Millisecond})
	if err != nil || page.Scanned != 1 || page.Complete || page.Progress == nil || len(page.Revisions) != 0 {
		t.Fatalf("time budget fabricated an introduction or lost progress: %#v %v", page, err)
	}
	next, err := rd.ScanHistory(context.Background(), guard.ID(), "table/orders", at, objectmodel.HistoryScanQuery{Limit: 10, Budget: 100, Progress: page.Progress})
	if err != nil || !next.Complete || len(next.Revisions) != 1 || next.Revisions[0].Commit != at {
		t.Fatalf("timed continuation lost actual introduction: %#v %v", next, err)
	}
	guard.commits = nil
	_, err = rd.ScanHistory(context.Background(), guard.ID(), "table/orders", at, objectmodel.HistoryScanQuery{Limit: 10, Budget: 100, TimeBudget: time.Nanosecond})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || len(guard.commits) != 0 {
		t.Fatalf("no-progress time budget returned successful empty page: %v %v", err, guard.commits)
	}
}
