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
	hookOutboxPending      atomic.Int64
	hookOutboxPendingAt    atomic.Int64
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
	meter := mp.Meter(ScopeName)
	if r.httpDuration, err = meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return nil, err
	}
	if r.httpActive, err = meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}")); err != nil {
		return nil, err
	}
	if r.opExecutions, err = meter.Int64Counter("kc.operation.executions", metric.WithUnit("{operation}")); err != nil {
		return nil, err
	}
	if r.opDuration, err = meter.Float64Histogram("kc.operation.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return nil, err
	}
	if r.opActive, err = meter.Int64UpDownCounter("kc.operation.active", metric.WithUnit("{operation}")); err != nil {
		return nil, err
	}
	if r.authenticationAttempts, err = meter.Int64Counter("kc.authentication.attempts", metric.WithUnit("{attempt}")); err != nil {
		return nil, err
	}
	if r.authenticationDuration, err = meter.Float64Histogram("kc.authentication.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return nil, err
	}
	if r.authDecisions, err = meter.Int64Counter("kc.authorization.decisions", metric.WithUnit("{decision}")); err != nil {
		return nil, err
	}
	if r.identityRequests, err = meter.Int64Counter("kc.identity.requests", metric.WithUnit("{request}")); err != nil {
		return nil, err
	}
	if r.workspaceDuration, err = meter.Float64Histogram("kc.workspace.resolve.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return nil, err
	}
	if r.workspaceMemberCount, err = meter.Int64Histogram("kc.workspace.member.count", metric.WithUnit("{repository}"), metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 25, 50, 100)); err != nil {
		return nil, err
	}
	if r.searchRequests, err = meter.Int64Counter("kc.search.requests", metric.WithUnit("{request}")); err != nil {
		return nil, err
	}
	if r.searchDuration, err = meter.Float64Histogram("kc.search.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return nil, err
	}
	if r.searchPhaseDuration, err = meter.Float64Histogram("kc.search.phase.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(searchPhaseDurationSecondsBuckets...)); err != nil {
		return nil, err
	}
	if r.searchCandidate, err = meter.Int64Histogram("kc.search.candidate.count", metric.WithUnit("{candidate}"), metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 20, 50, 100, 250, 500, 1000)); err != nil {
		return nil, err
	}
	if r.searchHydrated, err = meter.Int64Histogram("kc.search.hydrated.count", metric.WithUnit("{object}"), metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 20, 50, 100, 250, 500, 1000)); err != nil {
		return nil, err
	}
	if r.searchDropped, err = meter.Int64Histogram("kc.search.dropped.count", metric.WithUnit("{candidate}"), metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 20, 50, 100, 250, 500, 1000)); err != nil {
		return nil, err
	}
	if r.writerCommands, err = meter.Int64Counter("kc.writer.commands", metric.WithUnit("{command}")); err != nil {
		return nil, err
	}
	if r.writerDuration, err = meter.Float64Histogram("kc.writer.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return nil, err
	}
	if r.writerChangeCount, err = meter.Int64Histogram("kc.writer.change.count", metric.WithUnit("{change}"), metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 25, 50, 100, 250, 500, 1000)); err != nil {
		return nil, err
	}
	if r.writerPayloadSize, err = meter.Int64Histogram("kc.writer.payload.size", metric.WithUnit("By"), metric.WithExplicitBucketBoundaries(byteBuckets...)); err != nil {
		return nil, err
	}
	if r.projectionTransitions, err = meter.Int64Counter("kc.projection.transitions", metric.WithUnit("{transition}")); err != nil {
		return nil, err
	}
	if r.projectionDuration, err = meter.Float64Histogram("kc.projection.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 300)); err != nil {
		return nil, err
	}
	if r.projectionChangeCount, err = meter.Int64Histogram("kc.projection.change.count", metric.WithUnit("{document}"), metric.WithExplicitBucketBoundaries(countBuckets...)); err != nil {
		return nil, err
	}
	if _, err = meter.Int64ObservableGauge("kc.projection.lagging.count", metric.WithUnit("{projection}"), metric.WithInt64Callback(func(_ context.Context, observer metric.Int64Observer) error {
		provider, _ := r.projectionProvider.Load().(string)
		observer.Observe(r.projectionLagging.Load(), metric.WithAttributes(attribute.String("kc.retrieval.provider", provider)))
		return nil
	})); err != nil {
		return nil, err
	}
	if _, err = meter.Float64ObservableGauge("kc.projection.oldest_pending.age", metric.WithUnit("s"), metric.WithFloat64Callback(func(_ context.Context, observer metric.Float64Observer) error {
		provider, _ := r.projectionProvider.Load().(string)
		age := 0.0
		if pendingAt := r.projectionPendingAt.Load(); pendingAt > 0 {
			age = time.Since(time.Unix(0, pendingAt)).Seconds()
			if age < 0 {
				age = 0
			}
		}
		observer.Observe(age, metric.WithAttributes(attribute.String("kc.retrieval.provider", provider)))
		return nil
	})); err != nil {
		return nil, err
	}
	if _, err = meter.Int64ObservableGauge("kc.projection.documents", metric.WithUnit("{document}"), metric.WithInt64Callback(func(_ context.Context, observer metric.Int64Observer) error {
		provider, _ := r.projectionProvider.Load().(string)
		observer.Observe(r.projectionDocuments.Load(), metric.WithAttributes(attribute.String("kc.retrieval.provider", provider)))
		return nil
	})); err != nil {
		return nil, err
	}
	if r.bindingLookups, err = meter.Int64Counter("kc.binding.lookups", metric.WithUnit("{lookup}")); err != nil {
		return nil, err
	}
	if r.bindingDuration, err = meter.Float64Histogram("kc.binding.lookup.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return nil, err
	}
	if r.bindingObservationAge, err = meter.Float64Histogram("kc.binding.observation.age", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(observationAgeSecondsBuckets...)); err != nil {
		return nil, err
	}
	if r.evidenceAppends, err = meter.Int64Counter("kc.evidence.appends", metric.WithUnit("{append}")); err != nil {
		return nil, err
	}
	if r.evidenceDuration, err = meter.Float64Histogram("kc.evidence.append.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.0001, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1)); err != nil {
		return nil, err
	}
	if r.telemetryDropped, err = meter.Int64Counter("kc.telemetry.dropped", metric.WithUnit("{record}")); err != nil {
		return nil, err
	}
	if r.hookDispatches, err = meter.Int64Counter("kc.hook.dispatches", metric.WithUnit("{dispatch}")); err != nil {
		return nil, err
	}
	if r.hookDuration, err = meter.Float64Histogram("kc.hook.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return nil, err
	}
	if _, err = meter.Int64ObservableGauge("kc.hook.outbox.pending", metric.WithUnit("{event}"), metric.WithInt64Callback(func(_ context.Context, observer metric.Int64Observer) error {
		observer.Observe(r.hookOutboxPending.Load())
		return nil
	})); err != nil {
		return nil, err
	}
	if _, err = meter.Float64ObservableGauge("kc.hook.outbox.oldest_pending.age", metric.WithUnit("s"), metric.WithFloat64Callback(func(_ context.Context, observer metric.Float64Observer) error {
		age := 0.0
		if pendingAt := r.hookOutboxPendingAt.Load(); pendingAt > 0 {
			age = time.Since(time.Unix(0, pendingAt)).Seconds()
			if age < 0 {
				age = 0
			}
		}
		observer.Observe(age)
		return nil
	})); err != nil {
		return nil, err
	}
	if r.gateChecks, err = meter.Int64Counter("kc.gate.checks", metric.WithUnit("{check}")); err != nil {
		return nil, err
	}
	if r.gateDuration, err = meter.Float64Histogram("kc.gate.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(searchPhaseDurationSecondsBuckets...)); err != nil {
		return nil, err
	}
	if r.vfsTransferSize, err = meter.Int64Histogram("kc.vfs.transfer.size", metric.WithUnit("By"), metric.WithExplicitBucketBoundaries(byteBuckets...)); err != nil {
		return nil, err
	}
	if r.vfsDirectoryEntries, err = meter.Int64Histogram("kc.vfs.directory.entry.count", metric.WithUnit("{entry}"), metric.WithExplicitBucketBoundaries(countBuckets...)); err != nil {
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

func (r *Runtime) RecordHTTP(ctx context.Context, started time.Time, method, route string, status int, propagationOutcome string) {
	attrs := []attribute.KeyValue{
		attribute.String("http.request.method", enumValue(method, "OTHER", "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "OTHER")),
		attribute.String("http.route", enumValue(route, "unmatched", "/", "/health", "/livez", "/readyz", "/readyz/{surface}", "/metrics",
			"/catalog/v1/{operation}", "/knowledge/v1/{operation}", "/workspace-files/v1/{operation}", "/writer/v1/{operation}",
			"/governance/v1/{operation}", "/identity/v1/{operation}", "/admin/v1/{operation}", "/operations/v1/{operation}", "unmatched")),
		attribute.Int("http.response.status_code", status),
		attribute.String("kc.propagation.outcome", enumValue(propagationOutcome, "invalid", "accepted", "generated", "legacy", "invalid", "conflict")),
	}
	r.httpDuration.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(attrs...))
}

func (r *Runtime) AddHTTPActive(ctx context.Context, delta int64, method string) {
	r.httpActive.Add(ctx, delta, metric.WithAttributes(attribute.String("http.request.method", enumValue(method, "OTHER", "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "OTHER"))))
}

func (r *Runtime) StartAuthentication(ctx context.Context, provider string) (context.Context, trace.Span, time.Time) {
	provider = enumValue(provider, "other", "local", "gitea", "oidc", "taihu", "other")
	ctx, span := r.tracer.Start(ctx, "kc.authenticate", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(
		attribute.String("kc.identity.provider", provider),
	))
	return ctx, span, time.Now()
}

func (r *Runtime) EndAuthentication(ctx context.Context, span trace.Span, started time.Time, provider, outcome, errorType string) {
	attrs := []attribute.KeyValue{
		attribute.String("kc.identity.provider", enumValue(provider, "other", "local", "gitea", "oidc", "taihu", "other")),
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "denied", "invalid", "error")),
	}
	if errorType != "" && errorType != "none" {
		attrs = append(attrs, attribute.String("error.type", bounded(errorType, "other")))
	}
	r.authenticationAttempts.Add(ctx, 1, metric.WithAttributes(attrs...))
	r.authenticationDuration.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(attrs[:2]...))
	span.SetAttributes(attrs...)
	if outcome != "ok" {
		span.SetStatus(codes.Error, bounded(errorType, "other"))
	}
	span.End()
}

func (r *Runtime) RecordIdentity(ctx context.Context, provider, principalKind string, delegated bool) {
	attrs := []attribute.KeyValue{
		attribute.String("kc.identity.provider", enumValue(provider, "other", "local", "gitea", "oidc", "taihu", "other")),
		attribute.String("kc.principal.kind", enumValue(principalKind, "other", "owner", "user", "agent", "service", "other")),
		attribute.Bool("kc.identity.delegated", delegated),
	}
	r.identityRequests.Add(ctx, 1, metric.WithAttributes(attrs...))
	trace.SpanFromContext(ctx).SetAttributes(attrs...)
}

func (r *Runtime) RecordAuthorization(ctx context.Context, operation, decision string) {
	r.authDecisions.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kc.operation", bounded(operation, "other")),
		attribute.String("kc.authorization.decision", enumValue(decision, "deny", "allow", "deny")),
	))
}

func (r *Runtime) RecordWorkspaceResolve(ctx context.Context, outcome string, elapsed time.Duration, members int) {
	outcomeAttr := attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "partial", "unresolved", "denied", "invalid", "conflict", "error"))
	r.workspaceDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(outcomeAttr))
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(outcomeAttr)
	if members >= 0 {
		r.workspaceMemberCount.Record(ctx, int64(members), metric.WithAttributes(outcomeAttr))
		span.SetAttributes(attribute.Int("kc.workspace.member.count", members))
	}
}

func (r *Runtime) RecordSearch(ctx context.Context, provider, completeness, partialReason, outcome string, elapsed time.Duration, phases SearchPhases, candidates, hydrated, dropped, authorizationDropped int) {
	attrs := []attribute.KeyValue{
		attribute.String("kc.retrieval.provider", enumValue(provider, "other", "none", "opensearch", "other")),
		attribute.String("kc.search.completeness", enumValue(completeness, "other", "complete", "partial", "other")),
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "partial", "unresolved", "denied", "invalid", "conflict", "error")),
	}
	if completeness == "partial" {
		attrs = append(attrs, attribute.String("kc.search.partial_reason", enumValue(partialReason, "other", "authorization", "unsupported", "projection", "hydrate", "binding", "other")))
	}
	r.searchRequests.Add(ctx, 1, metric.WithAttributes(attrs...))
	r.searchDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attrs[0], attrs[1], attrs[2]))
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrs...)
	span.SetAttributes(
		attribute.Int("kc.search.candidate.count", candidates),
		attribute.Int("kc.search.hydrated.count", hydrated),
		attribute.Int("kc.search.dropped.count", dropped),
	)
	orchestration := elapsed - phases.Plan - phases.Probe - phases.Hydrate
	if orchestration < 0 {
		orchestration = 0
	}
	for phase, duration := range map[string]time.Duration{
		"plan": phases.Plan, "probe": phases.Probe, "hydrate": phases.Hydrate, "orchestration": orchestration,
	} {
		r.searchPhaseDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
			attrs[0], attrs[1], attrs[2], attribute.String("kc.search.phase", phase),
		))
		span.AddEvent("kc.search.phase", trace.WithAttributes(
			attribute.String("kc.search.phase", phase), attribute.Float64("kc.search.phase.duration.seconds", duration.Seconds()),
		))
	}
	r.searchCandidate.Record(ctx, int64(candidates), metric.WithAttributes(attrs[0]))
	r.searchHydrated.Record(ctx, int64(hydrated), metric.WithAttributes(attrs[0]))
	if authorizationDropped > dropped {
		authorizationDropped = dropped
	}
	if authorizationDropped > 0 {
		r.searchDropped.Record(ctx, int64(authorizationDropped), metric.WithAttributes(attrs[0], attribute.String("kc.search.partial_reason", "authorization")))
	}
	otherDropped := dropped - authorizationDropped
	if otherDropped > 0 {
		r.searchDropped.Record(ctx, int64(otherDropped), metric.WithAttributes(attrs[0], attribute.String("kc.search.partial_reason", "other")))
	}
	if dropped == 0 {
		r.searchDropped.Record(ctx, 0, metric.WithAttributes(attrs[0]))
	}
}

func (r *Runtime) RecordWriter(ctx context.Context, surface, outcome, errorType string, replayed bool, puts, removes, payloadBytes int, elapsed time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("kc.writer.surface", enumValue(surface, "other", "COMMIT", "PROPOSAL", "other")),
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "partial", "unresolved", "denied", "invalid", "conflict", "error")),
		attribute.Bool("kc.writer.replayed", replayed),
	}
	if errorType != "" && errorType != "none" {
		attrs = append(attrs, attribute.String("error.type", bounded(errorType, "other")))
	}
	r.writerCommands.Add(ctx, 1, metric.WithAttributes(attrs...))
	r.writerDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attrs[0], attrs[1]))
	changeAttrs := []attribute.KeyValue{attrs[0]}
	if puts > 0 {
		r.writerChangeCount.Record(ctx, int64(puts), metric.WithAttributes(append(changeAttrs, attribute.String("kc.writer.change.operation", "PUT"))...))
	}
	if removes > 0 {
		r.writerChangeCount.Record(ctx, int64(removes), metric.WithAttributes(append(changeAttrs, attribute.String("kc.writer.change.operation", "REMOVE"))...))
	}
	if payloadBytes >= 0 {
		r.writerPayloadSize.Record(ctx, int64(payloadBytes), metric.WithAttributes(attrs[0]))
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrs...)
	if puts >= 0 {
		span.SetAttributes(attribute.Int("kc.writer.put.count", puts), attribute.Int("kc.writer.remove.count", removes))
	}
	if payloadBytes >= 0 {
		span.SetAttributes(attribute.Int("kc.writer.payload.size", payloadBytes))
	}
}

func (r *Runtime) RecordProjection(ctx context.Context, provider, mode, outcome string, elapsed time.Duration, documents, updated, removed int) {
	providerAttr := attribute.String("kc.retrieval.provider", enumValue(provider, "other", "none", "opensearch", "other"))
	attrs := []attribute.KeyValue{
		providerAttr,
		attribute.String("kc.projection.mode", enumValue(mode, "other", "incremental", "rebuild", "ready", "other")),
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "partial", "unresolved", "denied", "invalid", "conflict", "error")),
	}
	r.projectionDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attrs...))
	if documents >= 0 {
		r.projectionDocuments.Store(int64(documents))
	}
	if updated >= 0 {
		r.projectionChangeCount.Record(ctx, int64(updated), metric.WithAttributes(providerAttr, attribute.String("kc.projection.change.operation", "update")))
	}
	if removed >= 0 {
		r.projectionChangeCount.Record(ctx, int64(removed), metric.WithAttributes(providerAttr, attribute.String("kc.projection.change.operation", "remove")))
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrs...)
	if documents >= 0 {
		span.SetAttributes(attribute.Int("kc.projection.document.count", documents), attribute.Int("kc.projection.updated.count", updated), attribute.Int("kc.projection.removed.count", removed))
	}
}

func (r *Runtime) StartBindingLookup(ctx context.Context, mode string) (context.Context, trace.Span, time.Time) {
	mode = enumValue(mode, "other", "state", "stream", "other")
	ctx, span := r.tracer.Start(ctx, "kc.binding.lookup", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(
		attribute.String("kc.binding.mode", mode),
	))
	return ctx, span, time.Now()
}

func (r *Runtime) EndBindingLookup(ctx context.Context, span trace.Span, started time.Time, mode, outcome, errorType string, observationAge time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("kc.binding.mode", enumValue(mode, "other", "state", "stream", "other")),
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "partial", "unresolved", "denied", "invalid", "conflict", "error")),
	}
	if errorType != "" && errorType != "none" {
		attrs = append(attrs, attribute.String("error.type", bounded(errorType, "other")))
	}
	r.bindingLookups.Add(ctx, 1, metric.WithAttributes(attrs...))
	r.bindingDuration.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(attrs[:2]...))
	if observationAge >= 0 {
		r.bindingObservationAge.Record(ctx, observationAge.Seconds(), metric.WithAttributes(attrs[0]))
		span.SetAttributes(attribute.Float64("kc.binding.observation.age.seconds", observationAge.Seconds()))
	}
	span.SetAttributes(attrs...)
	if outcome != "ok" {
		span.SetStatus(codes.Error, bounded(errorType, "other"))
	}
	span.End()
}

func (r *Runtime) SetProjectionBacklog(provider string, lagging int, oldestPendingAt time.Time) {
	provider = enumValue(provider, "other", "none", "opensearch", "other")
	r.projectionProvider.Store(provider)
	if lagging < 0 {
		lagging = 0
	}
	r.projectionLagging.Store(int64(lagging))
	if oldestPendingAt.IsZero() || lagging == 0 {
		r.projectionPendingAt.Store(0)
		return
	}
	r.projectionPendingAt.Store(oldestPendingAt.UnixNano())
}

func (r *Runtime) RecordEvidence(ctx context.Context, kind, outcome string, elapsed time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("kc.evidence.kind", enumValue(kind, "other", "access", "feedback", "system", "audit", "other")),
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "partial", "unresolved", "denied", "invalid", "conflict", "error")),
	}
	r.evidenceAppends.Add(ctx, 1, metric.WithAttributes(attrs...))
	r.evidenceDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attrs...))
	trace.SpanFromContext(ctx).AddEvent("kc.evidence.append", trace.WithAttributes(append(attrs, attribute.Float64("kc.evidence.append.duration.seconds", elapsed.Seconds()))...))
}

func (r *Runtime) RecordHook(ctx context.Context, phase, transport, outcome string, elapsed time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("kc.hook.phase", enumValue(phase, "other", "pre", "post", "other")),
		attribute.String("kc.hook.transport", enumValue(transport, "other", "exec", "http", "outbox", "other")),
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "error")),
	}
	r.hookDispatches.Add(ctx, 1, metric.WithAttributes(attrs...))
	r.hookDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attrs...))
	trace.SpanFromContext(ctx).AddEvent("kc.hook.dispatch", trace.WithAttributes(append(attrs, attribute.Float64("kc.hook.duration.seconds", elapsed.Seconds()))...))
	ended := time.Now()
	_, span := r.tracer.Start(ctx, "kc.hook.dispatch", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithTimestamp(ended.Add(-elapsed)), trace.WithAttributes(attrs...))
	if outcome != "ok" {
		span.SetStatus(codes.Error, "hook dispatch failed")
	}
	span.End(trace.WithTimestamp(ended))
}

func (r *Runtime) SetHookOutbox(pending int, oldestPendingAt time.Time) {
	if pending < 0 {
		pending = 0
	}
	r.hookOutboxPending.Store(int64(pending))
	if pending == 0 || oldestPendingAt.IsZero() {
		r.hookOutboxPendingAt.Store(0)
		return
	}
	r.hookOutboxPendingAt.Store(oldestPendingAt.UnixNano())
}

func (r *Runtime) RecordGate(ctx context.Context, required int, outcome string, elapsed time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "error")),
		attribute.Bool("kc.gate.required", required > 0),
	}
	r.gateChecks.Add(ctx, 1, metric.WithAttributes(attrs...))
	r.gateDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attrs...))
	trace.SpanFromContext(ctx).AddEvent("kc.gate.check", trace.WithAttributes(
		attrs[0], attrs[1], attribute.Int("kc.gate.requirement.count", required), attribute.Float64("kc.gate.duration.seconds", elapsed.Seconds()),
	))
	ended := time.Now()
	_, span := r.tracer.Start(ctx, "kc.gate.check", trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithTimestamp(ended.Add(-elapsed)), trace.WithAttributes(append(attrs, attribute.Int("kc.gate.requirement.count", required))...))
	if outcome != "ok" {
		span.SetStatus(codes.Error, "gate check failed")
	}
	span.End(trace.WithTimestamp(ended))
}

func (r *Runtime) RecordVFSVolume(ctx context.Context, operation, outcome string, transferredBytes, directoryEntries int) {
	attrs := []attribute.KeyValue{
		attribute.String("kc.operation", enumValue(operation, "other", "file-read", "file-list", "file-mounts", "other")),
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "denied", "invalid", "error")),
	}
	if transferredBytes >= 0 {
		r.vfsTransferSize.Record(ctx, int64(transferredBytes), metric.WithAttributes(attrs...))
	}
	if directoryEntries >= 0 {
		r.vfsDirectoryEntries.Record(ctx, int64(directoryEntries), metric.WithAttributes(attrs...))
	}
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.Int("kc.vfs.transferred.bytes", transferredBytes), attribute.Int("kc.vfs.directory.entry.count", directoryEntries),
	)
}

func bounded(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if len(value) > 64 {
		return "other"
	}
	return value
}

func enumValue(value, fallback string, allowed ...string) string {
	value = strings.TrimSpace(value)
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return fallback
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
