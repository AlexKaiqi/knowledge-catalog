package telemetry_test

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"kc/internal/telemetry"
)

func TestRuntimeExportsOTelInstrumentsThroughPrometheus(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-test", ServiceVersion: "test", TraceExporter: exporter})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })

	ctx, span, started := runtime.StartOperation(context.Background(), "knowledge", "read")
	if !span.SpanContext().IsValid() {
		t.Fatal("SDK-backed runtime must generate a valid span context")
	}
	runtime.EndOperation(ctx, span, started, "knowledge", "read", "ok", "")
	runtime.RecordEvidence(ctx, "access", "ok", 2*time.Millisecond)
	runtime.RecordEvidence(ctx, "retrieval", "ok", 2*time.Millisecond)
	runtime.RecordEvidence(ctx, "refine", "ok", 2*time.Millisecond)
	runtime.RecordWorkspaceResolve(ctx, "ok", 3*time.Millisecond, 2)
	runtime.RecordSearch(ctx, "none", "complete", "other", "ok", 4*time.Millisecond, telemetry.SearchPhases{
		Plan: time.Millisecond, Probe: time.Millisecond, Hydrate: time.Millisecond,
	}, 3, 2, 1, 0)
	runtime.RecordWriter(ctx, "COMMIT", "ok", "", false, 2, 1, 4096, 5*time.Millisecond)
	runtime.RecordProjection(ctx, "opensearch", "incremental", "ok", 8*time.Millisecond, 100, 3, 1)
	runtime.RecordHook(ctx, "pre", "exec", "ok", time.Millisecond)
	runtime.SetHookOutbox(2, time.Now().Add(-time.Second))
	runtime.RecordGate(ctx, 2, "ok", time.Millisecond)
	runtime.RecordIdentity(ctx, "local", "agent", false)
	runtime.RecordVFSVolume(ctx, "file-read", "ok", 2048, -1)
	bindingCtx, bindingSpan, bindingStarted := runtime.StartBindingLookup(ctx, "state")
	runtime.EndBindingLookup(bindingCtx, bindingSpan, bindingStarted, "state", "ok", "", time.Second)
	authCtx, authSpan, authStarted := runtime.StartAuthentication(ctx, "gitea")
	runtime.EndAuthentication(authCtx, authSpan, authStarted, "gitea", "ok", "")
	runtime.SetProjectionBacklog("opensearch", 2, time.Now().Add(-time.Second))
	ctx, otherSpan, otherStarted := runtime.StartOperation(context.Background(), "repo:high-cardinality", "read")
	runtime.EndOperation(ctx, otherSpan, otherStarted, "repo:high-cardinality", "read", "ok", "")
	if spans := exporter.GetSpans(); len(spans) != 6 || spans[0].Name != "kc.read" || spans[1].Name != "kc.hook.dispatch" || spans[2].Name != "kc.gate.check" || spans[3].Name != "kc.binding.lookup" || spans[4].Name != "kc.authenticate" {
		t.Fatalf("exported spans %#v", spans)
	}

	recorder := httptest.NewRecorder()
	runtime.MetricsHandler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	response := recorder.Result()
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	text := string(body)
	for _, want := range []string{
		"go_goroutines",
		"go_gc_duration_seconds",
		"kc_operation_executions_total",
		"kc_operation_duration_seconds",
		"kc_evidence_appends_total",
		"kc_workspace_resolve_duration_seconds",
		"kc_workspace_member_count",
		"kc_writer_change_count",
		"kc_writer_duration_seconds",
		"kc_writer_payload_size_bytes",
		"kc_search_candidate_count",
		"kc_search_phase_duration_seconds",
		"kc_search_hydrated_count",
		"kc_search_dropped_count",
		"kc_hook_dispatches_total",
		"kc_hook_duration_seconds",
		"kc_hook_outbox_pending",
		"kc_gate_checks_total",
		"kc_binding_lookups_total",
		"kc_binding_lookup_duration_seconds",
		"kc_binding_observation_age_seconds",
		"kc_authentication_attempts_total",
		"kc_identity_requests_total",
		"kc_vfs_transfer_size_bytes",
		"kc_projection_documents",
		"kc_projection_change_count",
		"kc_projection_lagging_count",
		"kc_projection_oldest_pending_age_seconds",
		`kc_operation="read"`,
		`kc_face="knowledge"`,
		`kc_face="other"`,
		`kc_evidence_kind="retrieval"`,
		`kc_evidence_kind="refine"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "repo:high-cardinality") {
		t.Fatalf("unknown enum value was not collapsed to other:\n%s", text)
	}
	if !strings.Contains(text, `kc_search_phase="probe"`) || !strings.Contains(text, `kc_search_duration_seconds_bucket{kc_outcome="ok"`) {
		t.Fatalf("search aggregation dimensions are missing:\n%s", text)
	}
	for _, boundary := range []string{`le="1.25"`, `le="1.5"`, `le="2"`, `le="3"`} {
		found := false
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, `kc_search_duration_seconds_bucket{`) && strings.Contains(line, boundary) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("SEARCH histogram is missing SLO-adjacent boundary %s:\n%s", boundary, text)
		}
	}
	for _, forbidden := range []string{"repository", "object_id", "principal", "request_id", "trace_id", "evidence_id"} {
		if strings.Contains(text, forbidden+"=") {
			t.Fatalf("high-cardinality metric label %q leaked:\n%s", forbidden, text)
		}
	}
}

func TestInvalidOptionalOTLPConfigDoesNotDisableRuntime(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "://invalid")
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-test", EnableOTLP: true})
	if err != nil {
		t.Fatalf("optional OTLP configuration changed runtime availability: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	if runtime.StartupError() == nil {
		t.Fatal("invalid exporter configuration was not surfaced")
	}
	recorder := httptest.NewRecorder()
	runtime.MetricsHandler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body, _ := io.ReadAll(recorder.Result().Body)
	if !strings.Contains(string(body), "kc_telemetry_dropped_total") || !strings.Contains(string(body), `kc_telemetry_drop_reason="export_error"`) {
		t.Fatalf("disabled exporter was not observable:\n%s", body)
	}
}

func TestExplicitZeroTraceRatioIsHonored(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	runtime, err := telemetry.New(telemetry.Config{
		ServiceName: "kc-test", TraceExporter: exporter,
		TraceRatio: 0, TraceRatioSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	ctx, span, started := runtime.StartOperation(context.Background(), "knowledge", "read")
	if !span.SpanContext().IsValid() || span.SpanContext().IsSampled() {
		t.Fatalf("explicit zero ratio produced unexpected span context %#v", span.SpanContext())
	}
	runtime.EndOperation(ctx, span, started, "knowledge", "read", "ok", "")
	if spans := exporter.GetSpans(); len(spans) != 0 {
		t.Fatalf("zero sampling ratio exported spans %#v", spans)
	}
}
