package plugin_test

import (
	"context"
	"fmt"
	"testing"

	"go.opentelemetry.io/otel/trace"
	kcclient "kc/client"
	"kc/kernel"
	"kc/liveaccess/evidence"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/server"
)

type auditedRead struct {
	requestID string
	traceID   trace.TraceID
	value     int64
}

// U17: both public audit selectors recover the adopted result after restart,
// even when one trace contains multiple reads of changing external state.
func TestServerLiveAccessAuditTraceAndRequestKeepOriginalValuesSeparate(t *testing.T) {
	one, two := newAcceptanceRuntime(t, 31), newAcceptanceRuntime(t, 22)
	f := newAcceptanceFixture(t, server.HTTPServerOptions{LiveAccessPlugins: acceptanceEntries(t, map[string]*acceptanceRuntime{"one": one, "two": two})})
	f.publish(t, acceptanceR1, "http://logical-source.invalid")
	commit := f.commits[acceptanceR1]
	reads := []auditedRead{
		{requestID: "original-read", traceID: trace.TraceID{15: 1}, value: 31},
		{requestID: "later-read", traceID: trace.TraceID{15: 1}, value: 37},
		{requestID: "other-trace-read", traceID: trace.TraceID{15: 2}, value: 59},
	}
	for _, read := range reads {
		one.value.Store(read.value)
		var delivered objectmodel.ObservedValue
		err := f.client(t, "reader").Objects().Read(auditTraceContext(read.traceID), kcclient.ObjectReadRequest{
			Reference: acceptanceReference(t, acceptanceR1, "/good"), Repository: string(acceptanceR1), Commit: string(commit),
		}, kcclient.RequestOptions{RequestID: read.requestID}, &delivered)
		if err != nil || delivered.Commit != commit || kernel.CanonicalDigest(delivered.Value) != kernel.CanonicalDigest(map[string]any{"value": read.value}) {
			t.Fatalf("read did not adopt the intended source value: %#v %v", delivered, err)
		}
	}
	one.value.Store(73)
	f.stop(t)
	one.server.Close()
	f.start(t)
	sourceCalls, sourceAttempts := one.calls.Load(), one.attempts.Load()
	queries := []struct {
		name  string
		query kcclient.AuditQueryRequest
		want  []auditedRead
	}{
		{"original-request", kcclient.AuditQueryRequest{RequestID: reads[0].requestID}, reads[:1]},
		{"later-request", kcclient.AuditQueryRequest{RequestID: reads[1].requestID}, reads[1:2]},
		{"shared-trace", kcclient.AuditQueryRequest{TraceID: reads[0].traceID.String()}, reads[:2]},
		{"other-trace", kcclient.AuditQueryRequest{TraceID: reads[2].traceID.String()}, reads[2:]},
		{"matching-selectors", kcclient.AuditQueryRequest{TraceID: reads[0].traceID.String(), RequestID: reads[1].requestID}, reads[1:2]},
		{"mismatched-selectors", kcclient.AuditQueryRequest{TraceID: reads[2].traceID.String(), RequestID: reads[0].requestID}, nil},
	}
	for _, query := range queries {
		t.Run(query.name, func(t *testing.T) {
			var page struct {
				Entries []liveservice.AuditEntry `json:"entries"`
			}
			query.query.Limit = 50
			// The audit request's transport trace must not replace its selectors.
			err := f.client(t, "auditor").OperationsService().AccessLog(auditTraceContext(trace.TraceID{15: 3}), query.query,
				kcclient.RequestOptions{RequestID: "audit-query-" + query.name}, &page)
			if err != nil || len(page.Entries) != len(query.want) {
				t.Fatalf("audit selectors returned the wrong original requests: %#v %v", page, err)
			}
			remaining := map[string]auditedRead{}
			for _, read := range query.want {
				remaining[read.requestID] = read
			}
			for _, entry := range page.Entries {
				read, ok := remaining[entry.RequestID]
				if !ok {
					t.Fatalf("audit repeated or substituted another request: %#v", entry)
				}
				delete(remaining, entry.RequestID)
				assertAuditedRead(t, entry, read, commit)
			}
		})
	}
	if one.calls.Load() != sourceCalls || one.attempts.Load() != sourceAttempts {
		t.Fatal("audit attempted to replace retained results from the offline source")
	}
}

func auditTraceContext(id trace.TraceID) context.Context {
	return trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: id, SpanID: trace.SpanID{7: 1}, TraceFlags: trace.FlagsSampled,
	}))
}

func assertAuditedRead(t *testing.T, entry liveservice.AuditEntry, want auditedRead, commit kernel.CommitID) {
	t.Helper()
	if entry.ResultFailure != nil || entry.Results == nil || entry.LiveAccess == nil || len(entry.Results.Units) != 1 || len(entry.Object) != 1 {
		t.Fatalf("audit did not restore one adopted unit and its access: %#v", entry)
	}
	if entry.Identity.Principal != "reader" || entry.Trace.TraceID != want.traceID.String() || entry.Results.Trace != entry.Trace ||
		entry.Results.RequestID != want.requestID || entry.Results.Identity != entry.Identity || entry.Object[0].ObjectRef.Commit != commit ||
		entry.Object[0].ObjectRef.Repository != acceptanceR1 || entry.Object[0].Locator != "/good" {
		t.Fatalf("retained result changed its caller, trace, request or fixed source: %#v", entry)
	}
	sample := entry.Results.Units[0]
	if sample.Outcome != evidence.ValueOutcome || sample.Failure != nil || sample.Observation == nil || sample.Commit != commit ||
		sample.Repository != acceptanceR1 || sample.Address.AspectName != "good" || sample.Address.ObjectID != acceptanceObject ||
		sample.Declaration.DeclarationDigest != sample.Observation.DeclarationDigest || sample.Observation.Basis.SourceRevision != fmt.Sprintf("source-%d", want.value) ||
		kernel.CanonicalDigest(sample.Value) != kernel.CanonicalDigest(map[string]any{"value": want.value}) {
		t.Fatalf("audit substituted another read's value or observation: %#v", sample)
	}
}
