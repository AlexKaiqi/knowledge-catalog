package cli

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"kc/internal/telemetry"
	"kc/observability"
	"kc/retrieval"
)

func TestObservedHTTPHandlerClosesSpanAndMetricsOnPanic(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-test", TraceExporter: exporter})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	handler := observedHTTPHandler(runtime, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("test panic payload must not become a metric label")
	}))
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("handler panic was swallowed")
			}
		}()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/panic-target", nil))
	}()
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Status.Code.String() != "Error" {
		t.Fatalf("panic span was not finalized as error: %#v", spans)
	}
	recorder := httptest.NewRecorder()
	runtime.MetricsHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, _ := io.ReadAll(recorder.Result().Body)
	text := string(body)
	if !strings.Contains(text, `http_response_status_code="500"`) || strings.Contains(text, "test panic payload") {
		t.Fatalf("panic HTTP telemetry is missing or leaked payload:\n%s", text)
	}
}

func TestManagedHTTPHandlerCloseIsIdempotent(t *testing.T) {
	handler := HTTPHandler(t.TempDir())
	closer, ok := handler.(interface{ Close() error })
	if !ok {
		t.Fatal("HTTP handler does not expose telemetry lifecycle")
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRunWithTelemetryCreatesCLIRootSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-test", TraceExporter: exporter})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	home := t.TempDir()
	result := RunWithTelemetry([]string{"local", "init", "--home", home, "--catalog", "kr://acme/catalog"}, runtime)
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "kc.init" {
		t.Fatalf("CLI root span %#v", spans)
	}
	events, err := readTrail(home, "kc", "local.init", 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("CLI audit %#v: %v", events, err)
	}
	if events[0].RequestID == "" || events[0].TraceID != spans[0].SpanContext.TraceID().String() || events[0].SpanID != spans[0].SpanContext.SpanID().String() {
		t.Fatalf("CLI audit is not correlated with root span: event %#v span %#v", events[0], spans[0].SpanContext)
	}
}

func TestTypedHTTPCollectsApplicationMetricsAndChildSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-test", TraceExporter: exporter})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	home := t.TempDir()
	if result := Run([]string{"local", "init", "--home", home, "--catalog", "kr://acme/telemetry"}); result.Status != 0 {
		t.Fatal(result.Stdout)
	}

	facade := &httpFacade{home: home, runtime: runtime, ready: newReadinessCache(home, 0)}
	mux := http.NewServeMux()
	facade.registerStatusRoutes(mux)
	facade.registerServiceRoutes(mux)
	handler := observedHTTPHandler(runtime, mux)
	t.Cleanup(func() { _ = facade.closeReadHome() })

	request := httptest.NewRequest(http.MethodPost, "/knowledge/v1/objects:read",
		bytes.NewBufferString(`{"workspace":"missing","object":"Policy:missing"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Kc-As", "agent:telemetry-test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("read without a grant returned %d: %s", response.Code, response.Body.String())
	}

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("typed HTTP request exported %d spans, want SERVER + application: %#v", len(spans), spans)
	}
	var serverSpan, operationSpan tracetest.SpanStub
	for _, span := range spans {
		switch span.Name {
		case "POST /knowledge/v1/{operation}":
			serverSpan = span
		case "kc.read":
			operationSpan = span
		}
	}
	if !serverSpan.SpanContext.IsValid() || !operationSpan.SpanContext.IsValid() ||
		serverSpan.SpanContext.TraceID() != operationSpan.SpanContext.TraceID() ||
		operationSpan.Parent.SpanID() != serverSpan.SpanContext.SpanID() {
		t.Fatalf("application span is not a child of the HTTP span: server=%#v operation=%#v", serverSpan, operationSpan)
	}

	events, err := observability.NewFileStore(home).Access(observability.AccessQuery{TraceID: operationSpan.SpanContext.TraceID().String()})
	if err != nil || len(events) != 1 {
		t.Fatalf("access evidence %#v: %v", events, err)
	}
	if events[0].Trace.SpanID != operationSpan.SpanContext.SpanID().String() || events[0].Decision != "DENY" {
		t.Fatalf("access evidence is not correlated with the denied application span: %#v", events[0])
	}

	metrics := httptest.NewRecorder()
	runtime.MetricsHandler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, _ := io.ReadAll(metrics.Result().Body)
	text := string(body)
	for _, want := range []string{
		"http_server_request_duration_seconds",
		"kc_operation_executions_total",
		`kc_operation="read"`,
		`kc_outcome="denied"`,
		"kc_authorization_decisions_total",
		`kc_authorization_decision="deny"`,
		"kc_evidence_appends_total",
		`kc_evidence_kind="access"`,
		`kc_evidence_kind="audit"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("typed HTTP metrics missing %q:\n%s", want, text)
		}
	}
}

func TestSearchTelemetryExportsPhaseAndVolumeFactsForAggregation(t *testing.T) {
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	result := retrieval.SearchResult{
		Completeness: retrieval.CompletenessComplete,
		Hits:         []retrieval.KnowledgeHit{},
		Stats: retrieval.SearchExecutionStats{
			Candidates: 7, Hydrated: 5, Dropped: 2,
			PlanDuration: 2 * time.Millisecond, ProbeDuration: 3 * time.Millisecond, HydrateDuration: 4 * time.Millisecond,
		},
	}
	recordDomainTelemetry(context.Background(), runtime, "search", map[string]FlagValue{"home": t.TempDir()}, &operationTelemetry{}, result, nil, 12*time.Millisecond)

	recorder := httptest.NewRecorder()
	runtime.MetricsHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, _ := io.ReadAll(recorder.Result().Body)
	text := string(body)
	for _, want := range []string{
		"kc_search_duration_seconds_bucket",
		"kc_search_phase_duration_seconds_bucket",
		`kc_search_phase="plan"`,
		`kc_search_phase="probe"`,
		`kc_search_phase="hydrate"`,
		`kc_search_phase="orchestration"`,
		"kc_search_candidate_count_sum",
		"kc_search_hydrated_count_sum",
		"kc_search_dropped_count_sum",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("search telemetry missing %q:\n%s", want, text)
		}
	}
}
