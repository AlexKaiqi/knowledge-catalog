package observability_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/observability"
)

func TestFileStoreTraceAndVersionedHitmap(t *testing.T) {
	store := &observability.FileStore{
		AccessPath:    filepath.Join(t.TempDir(), "access.jsonl"),
		FeedbackPath:  filepath.Join(t.TempDir(), "feedback.jsonl"),
		RetrievalPath: filepath.Join(t.TempDir(), "retrieval.jsonl"),
		RefinePath:    filepath.Join(t.TempDir(), "refine.jsonl"),
	}
	identity := observability.IdentityContext{Principal: "agent:finance", OnBehalfOf: "user:kai"}
	trace := observability.TraceContext{TraceID: "trace-1", SpanID: "span-1"}
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Metric:gmv", AspectName: "definition"}
	target := observability.KnowledgeAccess{
		KnowledgeRef: knowledge.PinnedKnowledgeRef{
			KnowledgeRef: knowledge.KnowledgeRef{Repository: "kr://acme/semantics", Object: "Metric:gmv"},
			Commit:       "commit-v1",
		},
		Address: &address,
	}
	for index, action := range []string{"search", "read"} {
		if _, err := store.RecordAccess(context.Background(), observability.AccessEvent{
			OccurredAt: []string{"2026-08-25T00:00:00.1Z", "2026-08-25T00:00:00.2Z"}[index],
			Identity:   identity, Trace: trace, Action: action, Decision: "ALLOW", Result: "RESOLVED",
			Knowledge: []observability.KnowledgeAccess{target},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.RecordFeedback(context.Background(), observability.FeedbackEvent{
		OccurredAt: "2026-08-25T00:00:00.2001Z",
		Identity:   identity, Trace: trace, Outcome: "helpful", Message: "used in the answer",
	}); err != nil {
		t.Fatal(err)
	}

	view, err := store.Trace("trace-1")
	if err != nil || len(view.Entries) != 3 || view.Entries[2].Kind != "feedback" {
		t.Fatalf("trace %#v %v", view, err)
	}
	hits, err := store.Hitmap(observability.AccessQuery{Object: "Metric:gmv"})
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits %#v %v", hits, err)
	}
	if hits[0].Hits != 2 || hits[0].KnowledgeRef.Commit != "commit-v1" || hits[0].Principals["agent:finance"] != 2 || hits[0].OnBehalfOf["user:kai"] != 2 {
		t.Fatal(hits[0])
	}

	target.KnowledgeRef.Commit = "commit-v2"
	if _, err := store.RecordAccess(context.Background(), observability.AccessEvent{
		OccurredAt: "2026-08-25T00:00:00.3Z",
		Identity:   identity, Trace: trace, Action: "read", Decision: "ALLOW", Result: "RESOLVED",
		Knowledge: []observability.KnowledgeAccess{target},
	}); err != nil {
		t.Fatal(err)
	}
	hits, err = store.Hitmap(observability.AccessQuery{Object: "Metric:gmv"})
	if err != nil || len(hits) != 2 {
		t.Fatalf("versioned hits %#v %v", hits, err)
	}
	if hits[0].KnowledgeRef.Commit != "commit-v1" || hits[0].Hits != 2 || hits[1].KnowledgeRef.Commit != "commit-v2" || hits[1].Hits != 1 {
		t.Fatal(hits)
	}
}

func TestIdentityAndTraceContextValidation(t *testing.T) {
	identity, err := (observability.PassThroughAuthenticator{}).Authenticate(observability.IdentityAssertion{
		Principal: "agent:a", OnBehalfOf: "user:u",
	})
	if err != nil || identity.Principal != "agent:a" || identity.OnBehalfOf != "user:u" {
		t.Fatal(err)
	}
	if err := (observability.IdentityContext{}).Validate(); err == nil {
		t.Fatal("principal is required")
	}
	if err := (observability.TraceContext{SpanID: "span-without-trace"}).Validate(); err == nil {
		t.Fatal("span without trace must fail")
	}
}

func TestAccessReceiptIsGeneratedOnlyAfterDurableAppend(t *testing.T) {
	store := observability.NewFileStore(t.TempDir())
	event := observability.AccessEvent{
		Identity: observability.IdentityContext{Principal: "agent:test"},
		Action:   "read", Decision: "ALLOW", Result: "RESOLVED",
	}
	id, err := store.RecordAccessReceipt(event)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "ev_") {
		t.Fatalf("unexpected evidence id %q", id)
	}
	page, err := store.Access(context.Background(), observability.AccessQuery{Limit: 10})
	if err != nil || len(page.Entries) != 1 {
		t.Fatalf("entries %#v: %v", page, err)
	}
	if page.Entries[0].EvidenceID != id {
		t.Fatalf("receipt %q does not identify persisted evidence %#v", id, page.Entries[0])
	}
	got, ok, err := store.GetAccess(context.Background(), id)
	if err != nil || !ok || got.EvidenceID != id {
		t.Fatalf("get after ack: %#v ok=%v err=%v", got, ok, err)
	}
	event.EvidenceID = "caller-controlled"
	if _, err := store.RecordAccessReceipt(event); err == nil {
		t.Fatal("caller-provided evidence id must be rejected")
	}
}

func TestFileStoreAccessQueryByTimeRepositoryPrincipalAndContinuation(t *testing.T) {
	store := observability.NewFileStore(t.TempDir())
	repoA := knowledge.PinnedKnowledgeRef{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: "kr://acme/semantics", Object: "Metric:gmv"},
		Commit:       "c1",
	}
	repoB := knowledge.PinnedKnowledgeRef{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: "kr://acme/runbooks", Object: "runbook/refund"},
		Commit:       "c1",
	}
	agent := observability.IdentityContext{Principal: "agent:finance"}
	user := observability.IdentityContext{Principal: "user:kai"}
	write := func(at string, identity observability.IdentityContext, ref knowledge.PinnedKnowledgeRef) string {
		t.Helper()
		id, err := store.RecordAccessReceipt(observability.AccessEvent{
			OccurredAt: at, Identity: identity, Action: "read", Decision: "ALLOW", Result: "RESOLVED",
			Knowledge: []observability.KnowledgeAccess{{KnowledgeRef: ref}},
		})
		if err != nil {
			t.Fatal(err)
		}
		got, ok, err := store.GetAccess(context.Background(), id)
		if err != nil || !ok || got.OccurredAt != at {
			t.Fatalf("ack was not visible via GetAccess: %#v ok=%v err=%v", got, ok, err)
		}
		return id
	}
	idEarly := write("2026-08-25T00:00:00Z", agent, repoA)
	idMid := write("2026-08-25T01:00:00Z", user, repoA)
	write("2026-08-25T02:00:00Z", agent, repoB)
	idLate := write("2026-08-26T00:00:00Z", agent, repoA)

	window, err := store.Access(context.Background(), observability.AccessQuery{
		Since: "2026-08-25T00:00:00Z", Until: "2026-08-25T01:00:00Z", Repository: "kr://acme/semantics",
	})
	if err != nil || len(window.Entries) != 2 {
		t.Fatalf("time+repo window %#v err=%v", window, err)
	}
	if window.Entries[0].EvidenceID != idEarly || window.Entries[1].EvidenceID != idMid {
		t.Fatalf("window order %#v", window.Entries)
	}

	subject, err := store.Access(context.Background(), observability.AccessQuery{
		Principal: "agent:finance", Repository: "kr://acme/semantics",
	})
	if err != nil || len(subject.Entries) != 2 {
		t.Fatalf("principal+repo %#v err=%v", subject, err)
	}
	if subject.Entries[0].EvidenceID != idEarly || subject.Entries[1].EvidenceID != idLate {
		t.Fatalf("principal+repo order %#v", subject.Entries)
	}

	first, err := store.Access(context.Background(), observability.AccessQuery{Repository: "kr://acme/semantics", Limit: 1})
	if err != nil || len(first.Entries) != 1 || first.Entries[0].EvidenceID != idLate || first.Exhausted || first.Continuation == "" {
		t.Fatalf("newest page %#v err=%v", first, err)
	}
	if first.CompleteThrough != "2026-08-26T00:00:00Z" {
		t.Fatalf("watermark %q", first.CompleteThrough)
	}
	second, err := store.Access(context.Background(), observability.AccessQuery{
		Repository: "kr://acme/semantics", Limit: 1, Continuation: first.Continuation,
	})
	if err != nil || len(second.Entries) != 1 || second.Entries[0].EvidenceID != idMid || second.Exhausted {
		t.Fatalf("older page %#v err=%v", second, err)
	}
	third, err := store.Access(context.Background(), observability.AccessQuery{
		Repository: "kr://acme/semantics", Limit: 1, Continuation: second.Continuation,
	})
	if err != nil || len(third.Entries) != 1 || third.Entries[0].EvidenceID != idEarly || !third.Exhausted || third.Continuation != "" {
		t.Fatalf("oldest page %#v err=%v", third, err)
	}

	hits, err := store.Hitmap(observability.AccessQuery{
		Since: "2026-08-25T00:00:00Z", Until: "2026-08-25T01:00:00Z", Repository: "kr://acme/semantics",
	})
	if err != nil || len(hits) != 1 || hits[0].Hits != 2 || hits[0].Principals["agent:finance"] != 1 || hits[0].Principals["user:kai"] != 1 {
		t.Fatalf("windowed hitmap %#v err=%v", hits, err)
	}

	if _, err := store.Access(context.Background(), observability.AccessQuery{Since: "not-a-time"}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("invalid since: %v", err)
	}
	if _, err := store.Access(context.Background(), observability.AccessQuery{Continuation: "not-a-cursor"}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("invalid continuation: %v", err)
	}
}
