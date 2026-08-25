package observability_test

import (
	"path/filepath"
	"testing"

	"kc/knowledge"
	"kc/observability"
)

func TestFileStoreTraceAndVersionedHitmap(t *testing.T) {
	store := &observability.FileStore{
		AccessPath:   filepath.Join(t.TempDir(), "access.jsonl"),
		FeedbackPath: filepath.Join(t.TempDir(), "feedback.jsonl"),
	}
	identity := observability.IdentityContext{Principal: "agent:finance", OnBehalfOf: "user:kai"}
	trace := observability.TraceContext{TraceID: "trace-1", SpanID: "span-1", SessionID: "session-1"}
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Metric:gmv", AspectName: "definition"}
	target := observability.KnowledgeAccess{
		KnowledgeRef: knowledge.PinnedKnowledgeRef{
			KnowledgeRef: knowledge.KnowledgeRef{Repository: "kr://acme/semantics", Object: "Metric:gmv"},
			Commit:       "commit-v1",
		},
		Address: &address,
	}
	for index, action := range []string{"search", "read"} {
		if err := store.RecordAccess(observability.AccessEvent{
			OccurredAt: []string{"2026-08-25T00:00:00.1Z", "2026-08-25T00:00:00.2Z"}[index],
			Identity:   identity, Trace: trace, Action: action, Decision: "ALLOW", Result: "RESOLVED",
			Knowledge: []observability.KnowledgeAccess{target},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.RecordFeedback(observability.FeedbackEvent{
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
	if err := store.RecordAccess(observability.AccessEvent{
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
