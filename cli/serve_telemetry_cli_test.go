package cli

import (
	"context"
	"encoding/json"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	kchome "kc/home"
	"kc/internal/telemetry"
	"kc/internal/testkit"
	inv "kc/invocation"
	"os"
	"path/filepath"
	"testing"
)

// testHTTPFacade rebuilds the serving facade through the public assembly path
// (httpFacade became server-private in CAP-ORG-01 B6).
func TestRunWithTelemetryCreatesCLIRootSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-test", TraceExporter: exporter})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	root := t.TempDir()
	home := filepath.Join(root, "durable")
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	telemetryFake := testkit.NewLakeFSFake(t)
	cfg := kchome.DeploymentConfig{Version: 1, StateDir: home, CacheDir: filepath.Join(root, "cache"), Auth: "local", BootstrapPrincipal: "agent:operator", Repositories: []kchome.RepositoryBinding{{ID: "kr://kc/system", Driver: "lakefs", DSN: telemetryFake.DSN(telemetryFake.NewRepo())}}, Registry: kchome.RegistryBinding{ID: "kr://kc/registry", Driver: "lakefs", DSN: telemetryFake.DSN(telemetryFake.NewRepo())}}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "deployment.json")
	if err := os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	result := RunWithTelemetry([]string{"deployment", "init", "--config", configPath}, runtime)
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "kc.deployment-init" {
		t.Fatalf("CLI root span %#v", spans)
	}
	events, err := inv.ReadTrail(home, "kc", "deployment.init", 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("CLI audit %#v: %v", events, err)
	}
	if events[0].RequestID == "" || events[0].TraceID != spans[0].SpanContext.TraceID().String() || events[0].SpanID != spans[0].SpanContext.SpanID().String() {
		t.Fatalf("CLI audit is not correlated with root span: event %#v span %#v", events[0], spans[0].SpanContext)
	}
}
