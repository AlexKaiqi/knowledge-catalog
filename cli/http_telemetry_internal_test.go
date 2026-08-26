package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"kc/internal/telemetry"
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
