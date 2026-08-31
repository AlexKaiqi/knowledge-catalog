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
	runtime.RecordWorkspaceResolve(ctx, "ok", 3*time.Millisecond, 2)
	runtime.RecordSearch(ctx, "none", "complete", "other", "ok", 4*time.Millisecond, 3, 2, 1, 0)
	runtime.RecordWriter(ctx, "COMMIT", "ok", "", false, 2, 1)
	runtime.RecordHook(ctx, "pre", "exec", "ok")
	runtime.SetProjectionBacklog("opensearch", 2, time.Now().Add(-time.Second))
	ctx, otherSpan, otherStarted := runtime.StartOperation(context.Background(), "repo:high-cardinality", "read")
	runtime.EndOperation(ctx, otherSpan, otherStarted, "repo:high-cardinality", "read", "ok", "")
	if spans := exporter.GetSpans(); len(spans) != 2 || spans[0].Name != "kc.read" {
		t.Fatalf("exported spans %#v", spans)
	}

	recorder := httptest.NewRecorder()
	runtime.MetricsHandler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	response := recorder.Result()
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	text := string(body)
	for _, want := range []string{
		"kc_operation_executions_total",
		"kc_operation_duration_seconds",
		"kc_evidence_appends_total",
		"kc_workspace_resolve_duration_seconds",
		"kc_workspace_member_count",
		"kc_writer_change_count",
		"kc_search_candidate_count",
		"kc_search_hydrated_count",
		"kc_search_dropped_count",
		"kc_hook_dispatches_total",
		"kc_projection_lagging_count",
		"kc_projection_oldest_pending_age_seconds",
		`kc_operation="read"`,
		`kc_face="knowledge"`,
		`kc_face="other"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "repo:high-cardinality") {
		t.Fatalf("unknown enum value was not collapsed to other:\n%s", text)
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
