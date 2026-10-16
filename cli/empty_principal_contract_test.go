package cli

import (
	"context"
	"path/filepath"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"kc/internal/telemetry"
	inv "kc/invocation"
)

func TestDebtLocalFixtureRetainsTelemetryAndEvidence(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "local-owner-debt", TraceExporter: exporter})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	dir := filepath.Join(t.TempDir(), "local")
	result, handled := fixtureHomeOperation([]string{"local", "init", "--home", dir}, runtime)
	if !handled || result.Status != 0 {
		t.Fatalf("local fixture stopped working: handled=%v result=%#v", handled, result)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "kc.local-init" {
		t.Fatalf("local fixture lost operation telemetry: %#v", spans)
	}
	events, err := inv.ReadTrail(dir, "kc", "local.init", 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("local fixture lost audit: events=%#v error=%v", events, err)
	}
	if events[0].RequestID == "" || events[0].TraceID != spans[0].SpanContext.TraceID().String() || events[0].SpanID != spans[0].SpanContext.SpanID().String() {
		t.Fatalf("local evidence lost telemetry correlation: %#v", events[0])
	}
}
