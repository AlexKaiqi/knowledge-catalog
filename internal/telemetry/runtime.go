// Package telemetry owns the process-local diagnostic telemetry runtime.
//
// It deliberately contains no knowledge-catalog domain types. Callers translate
// domain outcomes into the bounded attributes accepted here, which keeps the
// metrics backend free of repository, object, workspace, principal, and query
// cardinality.
package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	SchemaVersion = "1.0"
	ScopeName     = "kc/internal/telemetry"
)

type Config struct {
	ServiceName    string
	ServiceVersion string
	InstanceID     string
	TraceRatio     float64
	// TraceRatioSet distinguishes an explicit zero (never sample) from the
	// default zero value, which selects the reference profile ratio of one.
	TraceRatioSet bool
	// TraceExporter is primarily an embedding/test seam. When absent and
	// EnableOTLP is true, standard OTEL_EXPORTER_OTLP[_TRACES]_ENDPOINT
	// environment variables enable the OTLP/HTTP exporter.
	TraceExporter sdktrace.SpanExporter
	// LogExporter is the log equivalent of TraceExporter. The reference
	// runtime emits one bounded completion event at each product HTTP boundary.
	LogExporter sdklog.Exporter
	EnableOTLP  bool
}

// SearchPhases are aggregate timings captured at the real executor boundaries
// for one SEARCH. They are diagnostic facts, not public result protocol.
type SearchPhases struct {
	Plan    time.Duration
	Probe   time.Duration
	Hydrate time.Duration
}

// Runtime is owned by one process or one HTTP handler. It does not install
// global OpenTelemetry providers, so tests and embedded users remain isolated.
type Runtime struct {
	registry       *promclient.Registry
	metricProvider *sdkmetric.MeterProvider
	traceProvider  *sdktrace.TracerProvider
	logProvider    *sdklog.LoggerProvider
	tracer         trace.Tracer
	logger         otellog.Logger
	propagator     propagation.TextMapPropagator

	httpDuration           metric.Float64Histogram
	httpActive             metric.Int64UpDownCounter
	opExecutions           metric.Int64Counter
	opDuration             metric.Float64Histogram
	opActive               metric.Int64UpDownCounter
	authenticationAttempts metric.Int64Counter
	authenticationDuration metric.Float64Histogram
	authDecisions          metric.Int64Counter
	identityRequests       metric.Int64Counter
	workspaceDuration      metric.Float64Histogram
	workspaceMemberCount   metric.Int64Histogram
	snapshotOperations     metric.Int64Counter
	snapshotDuration       metric.Float64Histogram
	snapshotActive         metric.Int64UpDownCounter
	snapshotBytes          metric.Int64Histogram
	readObjectCount        metric.Int64Histogram
	readUnitCount          metric.Int64Histogram
	searchRequests         metric.Int64Counter
	searchDuration         metric.Float64Histogram
	searchPhaseDuration    metric.Float64Histogram
	searchCandidate        metric.Int64Histogram
	searchHydrated         metric.Int64Histogram
	searchDropped          metric.Int64Histogram
	writerCommands         metric.Int64Counter
	writerDuration         metric.Float64Histogram
	writerChangeCount      metric.Int64Histogram
	writerPayloadSize      metric.Int64Histogram
	projectionTransitions  metric.Int64Counter
	projectionDuration     metric.Float64Histogram
	projectionChangeCount  metric.Int64Histogram
	bindingLookups         metric.Int64Counter
	bindingDuration        metric.Float64Histogram
	bindingObservationAge  metric.Float64Histogram
	evidenceAppends        metric.Int64Counter
	evidenceDuration       metric.Float64Histogram
	evidenceBytes          metric.Int64Histogram
	telemetryDropped       metric.Int64Counter
	hookDispatches         metric.Int64Counter
	hookDuration           metric.Float64Histogram
	gateChecks             metric.Int64Counter
	gateDuration           metric.Float64Histogram
	vfsTransferSize        metric.Int64Histogram
	vfsDirectoryEntries    metric.Int64Histogram
	projectionLagging      atomic.Int64
	projectionPendingAt    atomic.Int64
	projectionDocuments    atomic.Int64
	projectionProvider     atomic.Value
	projectionBacklogSet   atomic.Bool
	hookOutboxPending      atomic.Int64
	hookOutboxPendingAt    atomic.Int64
	evidenceUsedMilli      atomic.Int64
	startupErr             error
}

func New(cfg Config) (*Runtime, error) {
	if strings.TrimSpace(cfg.ServiceName) == "" {
		cfg.ServiceName = "kc"
	}
	if strings.TrimSpace(cfg.InstanceID) == "" {
		cfg.InstanceID = newToken("instance")
	}
	if strings.TrimSpace(cfg.ServiceVersion) == "" {
		cfg.ServiceVersion = buildVersion()
	}
	if cfg.TraceRatio < 0 || cfg.TraceRatio > 1 {
		return nil, fmt.Errorf("trace ratio must be between 0 and 1")
	}
	if cfg.TraceRatio == 0 && !cfg.TraceRatioSet {
		// Local reference profile: keep spans in-process so every accepted HTTP
		// request gets valid W3C identifiers even when no exporter is configured.
		cfg.TraceRatio = 1
	}
	res, err := resource.New(context.Background(), resource.WithSchemaURL(semconv.SchemaURL), resource.WithAttributes(
		semconv.ServiceNamespace("knowledge-catalog"),
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.ServiceInstanceID(cfg.InstanceID),
		attribute.String("kc.telemetry.schema.version", SchemaVersion),
	))
	if err != nil {
		return nil, err
	}
	registry := promclient.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	exporter, err := otelprom.New(otelprom.WithRegisterer(registry))
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter), sdkmetric.WithResource(res))
	traceOptions := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.TraceRatio))),
	}
	var startupErr error
	if cfg.TraceExporter != nil {
		traceOptions = append(traceOptions, sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(cfg.TraceExporter)))
	} else if cfg.EnableOTLP && otlpEndpointConfigured("traces") {
		if exportErr := validateOTLPEndpoint("traces"); exportErr != nil {
			// Diagnostic telemetry is best effort. A malformed exporter config
			// disables that exporter but must not disable the KC protocol surface.
			startupErr = fmt.Errorf("initialize OTLP trace exporter: %w", exportErr)
		} else {
			exporter, exportErr := otlptracehttp.New(context.Background())
			if exportErr != nil {
				startupErr = fmt.Errorf("initialize OTLP trace exporter: %w", exportErr)
			} else {
				traceOptions = append(traceOptions, sdktrace.WithBatcher(exporter))
			}
		}
	}
	tp := sdktrace.NewTracerProvider(traceOptions...)
	logOptions := []sdklog.LoggerProviderOption{sdklog.WithResource(res)}
	if cfg.LogExporter != nil {
		logOptions = append(logOptions, sdklog.WithProcessor(sdklog.NewSimpleProcessor(cfg.LogExporter)))
	} else if cfg.EnableOTLP && otlpEndpointConfigured("logs") {
		if exportErr := validateOTLPEndpoint("logs"); exportErr != nil {
			startupErr = errors.Join(startupErr, fmt.Errorf("initialize OTLP log exporter: %w", exportErr))
		} else {
			exporter, exportErr := otlploghttp.New(context.Background())
			if exportErr != nil {
				startupErr = errors.Join(startupErr, fmt.Errorf("initialize OTLP log exporter: %w", exportErr))
			} else {
				logOptions = append(logOptions, sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)))
			}
		}
	}
	lp := sdklog.NewLoggerProvider(logOptions...)
	r := &Runtime{
		registry: registry, metricProvider: mp, traceProvider: tp, logProvider: lp,
		tracer: tp.Tracer(ScopeName), logger: lp.Logger(ScopeName), propagator: propagation.TraceContext{}, startupErr: startupErr,
	}
	r.projectionProvider.Store("other")
	r.evidenceUsedMilli.Store(-1)
	meter := mp.Meter(ScopeName)
	if err := r.registerInstruments(meter); err != nil {
		return nil, err
	}
	if startupErr != nil {
		signal := "trace"
		if strings.Contains(startupErr.Error(), "log exporter") {
			signal = "log"
		}
		r.telemetryDropped.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("kc.telemetry.signal", signal),
			attribute.String("kc.telemetry.drop_reason", "export_error"),
		))
	}
	return r, nil
}

// StartupError reports a disabled optional telemetry exporter. The runtime and
// protocol surface remain usable; callers may expose this via management logs.
func (r *Runtime) StartupError() error {
	if r == nil {
		return nil
	}
	return r.startupErr
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}
	metricErr := r.metricProvider.Shutdown(ctx)
	traceErr := r.traceProvider.Shutdown(ctx)
	logErr := r.logProvider.Shutdown(ctx)
	if metricErr != nil {
		return metricErr
	}
	return errors.Join(traceErr, logErr)
}

func (r *Runtime) ForceFlush(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if err := r.metricProvider.ForceFlush(ctx); err != nil {
		return err
	}
	return errors.Join(r.traceProvider.ForceFlush(ctx), r.logProvider.ForceFlush(ctx))
}

func (r *Runtime) MetricsHandler() http.Handler {
	if r == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}

func (r *Runtime) Propagator() propagation.TextMapPropagator { return r.propagator }

func (r *Runtime) StartServer(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return r.tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attrs...))
}

func (r *Runtime) StartOperation(ctx context.Context, face, operation string) (context.Context, trace.Span, time.Time) {
	face = enumValue(face, "other", "catalog", "knowledge", "writer", "projection", "vfs", "control", "hook", "gate", "other")
	operation = bounded(operation, "other")
	attrs := []attribute.KeyValue{attribute.String("kc.face", face), attribute.String("kc.operation", operation)}
	r.opActive.Add(ctx, 1, metric.WithAttributes(attrs...))
	ctx, span := r.tracer.Start(ctx, "kc."+operation, trace.WithSpanKind(trace.SpanKindInternal), trace.WithAttributes(attrs...))
	return ctx, span, time.Now()
}

func (r *Runtime) EndOperation(ctx context.Context, span trace.Span, started time.Time, face, operation, outcome, errorType string) {
	base := []attribute.KeyValue{
		attribute.String("kc.face", enumValue(face, "other", "catalog", "knowledge", "writer", "projection", "vfs", "control", "hook", "gate", "other")),
		attribute.String("kc.operation", bounded(operation, "other")),
	}
	durationAttrs := append(append([]attribute.KeyValue{}, base...),
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "partial", "unresolved", "denied", "invalid", "conflict", "error")),
	)
	executionAttrs := append([]attribute.KeyValue{}, durationAttrs...)
	if errorType != "" && errorType != "none" {
		executionAttrs = append(executionAttrs, attribute.String("error.type", bounded(errorType, "other")))
	}
	r.opActive.Add(ctx, -1, metric.WithAttributes(base...))
	r.opExecutions.Add(ctx, 1, metric.WithAttributes(executionAttrs...))
	r.opDuration.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(durationAttrs...))
	span.SetAttributes(executionAttrs...)
	if outcome != "ok" && outcome != "partial" {
		span.SetStatus(codes.Error, bounded(errorType, "other"))
	}
	span.End()
}

func NewID(prefix string) string { return newToken(prefix) }

func newToken(prefix string) string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(raw)
}

func otlpEndpointConfigured(signal string) bool {
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_"+strings.ToUpper(signal)+"_ENDPOINT")) != "" ||
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != ""
}

func validateOTLPEndpoint(signal string) error {
	raw := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_" + strings.ToUpper(signal) + "_ENDPOINT"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	}
	endpoint, err := url.ParseRequestURI(raw)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return fmt.Errorf("endpoint must be an absolute http(s) URL")
	}
	return nil
}

func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && strings.TrimSpace(info.Main.Version) != "" {
		return info.Main.Version
	}
	return "unknown"
}
