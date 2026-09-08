package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func mergeTestHit(id string, order any) retrieval.KnowledgeHit {
	return retrieval.KnowledgeHit{Knowledge: knowledge.KnowledgeValue{Address: knowledge.Address{ObjectID: knowledge.ObjectID(id)}}, Evidence: []retrieval.LaneEvidence{{ProviderOrder: []any{order}}}}
}

func TestWorkspaceOrderUsesDeclaredScalarWithoutLoss(t *testing.T) {
	req := retrieval.SearchOf(retrieval.SearchEXISTS("value"), retrieval.SearchSORT("value", "asc"))
	for _, test := range []struct {
		kind string
		a, b any
	}{
		{"long", json.Number("9007199254740992"), json.Number("9007199254740993")},
		{"timestamp", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00.000000001Z"},
	} {
		if !workspaceHitLess(mergeTestHit("z", test.a), mergeTestHit("a", test.b), req, test.kind) {
			t.Fatalf("lost %s order", test.kind)
		}
	}
}

func TestWorkspaceBuffersAndResumesConsumedOffset(t *testing.T) {
	calls := 0
	fetch := func(_ context.Context, c *workspaceSearchCursor) (retrieval.SearchResult, error) {
		calls++
		start, _ := strconv.Atoi(c.position)
		result := retrieval.SearchResult{Completeness: retrieval.CompletenessComplete}
		for n := start; n < start+workspaceSearchBatchSize && n < 21; n++ {
			result.Hits = append(result.Hits, mergeTestHit(fmt.Sprint(n), nil))
		}
		if start+len(result.Hits) < 21 {
			result.Continuation = strconv.Itoa(start + len(result.Hits))
		}
		return result, nil
	}
	cursor := workspaceSearchCursor{}
	if err := fillWorkspaceHead(context.Background(), &cursor, fetch); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 5; n++ {
		cursor.consumeHead()
		if err := fillWorkspaceHead(context.Background(), &cursor, fetch); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("fetched %d times for five buffered hits", calls)
	}
	resumed := workspaceSearchCursor{position: cursor.position, offset: cursor.offset}
	if err := fillWorkspaceHead(context.Background(), &resumed, fetch); err != nil {
		t.Fatal(err)
	}
	if resumed.head.Knowledge.Address.ObjectID != "5" {
		t.Fatalf("resume skipped/repeated: %#v", resumed.head)
	}
	for !resumed.exhausted {
		resumed.consumeHead()
		if err := fillWorkspaceHead(context.Background(), &resumed, fetch); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 3 {
		t.Fatalf("expected one replay plus one remaining batch, calls=%d", calls)
	}
}

func TestWorkspaceInitialFetchConcurrencyIsBounded(t *testing.T) {
	cursors := make([]workspaceSearchCursor, 11)
	var active, peak atomic.Int32
	fetch := func(_ context.Context, _ *workspaceSearchCursor) (retrieval.SearchResult, error) {
		now := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); now > old && !peak.CompareAndSwap(old, now); old = peak.Load() {
		}
		time.Sleep(10 * time.Millisecond)
		return retrieval.SearchResult{Completeness: retrieval.CompletenessComplete}, nil
	}
	if err := primeWorkspaceHeads(context.Background(), cursors, fetch); err != nil {
		t.Fatal(err)
	}
	if peak.Load() < 2 || peak.Load() > workspaceSearchConcurrency {
		t.Fatalf("peak concurrency=%d", peak.Load())
	}
}

func TestWorkspacePartialUnknownHeadStopsMerge(t *testing.T) {
	cursor := workspaceSearchCursor{spec: retrieval.AccessSpec{Repository: kernel.RepositoryID("kr://a")}}
	err := fillWorkspaceHead(context.Background(), &cursor, func(context.Context, *workspaceSearchCursor) (retrieval.SearchResult, error) {
		return retrieval.SearchResult{Completeness: retrieval.CompletenessPartial, Continuation: "resume"}, nil
	})
	if err != nil || !cursor.blocked || cursor.exhausted || cursor.position != "resume" {
		t.Fatalf("unknown head treated exhausted: %#v %v", cursor, err)
	}
}

func TestWorkspaceResumeAcrossShortBudgetBatch(t *testing.T) {
	cursor := workspaceSearchCursor{position: "0", offset: 5}
	err := fillWorkspaceHead(context.Background(), &cursor, func(context.Context, *workspaceSearchCursor) (retrieval.SearchResult, error) {
		return retrieval.SearchResult{Completeness: retrieval.CompletenessPartial, Hits: []retrieval.KnowledgeHit{mergeTestHit("0", nil), mergeTestHit("1", nil)}, Continuation: "2"}, nil
	})
	if err != nil || cursor.offset != 3 || cursor.position != "2" || !cursor.blocked {
		t.Fatalf("replay progress lost: %#v %v", cursor, err)
	}
	cursor.blocked = false
	err = fillWorkspaceHead(context.Background(), &cursor, func(context.Context, *workspaceSearchCursor) (retrieval.SearchResult, error) {
		return retrieval.SearchResult{Completeness: retrieval.CompletenessComplete, Hits: []retrieval.KnowledgeHit{mergeTestHit("2", nil), mergeTestHit("3", nil), mergeTestHit("4", nil), mergeTestHit("5", nil)}}, nil
	})
	if err != nil || cursor.head.Knowledge.Address.ObjectID != "5" {
		t.Fatalf("replay resumed incorrectly: %#v %v", cursor, err)
	}
}
