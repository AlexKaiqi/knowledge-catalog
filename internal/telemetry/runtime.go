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
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
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
	EnableOTLP    bool
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
	tracer         trace.Tracer
	propagator     propagation.TextMapPropagator

	httpDuration          metric.Float64Histogram
	httpActive            metric.Int64UpDownCounter
	opExecutions          metric.Int64Counter
	opDuration            metric.Float64Histogram
	opActive              metric.Int64UpDownCounter
	authDecisions         metric.Int64Counter
	workspaceDuration     metric.Float64Histogram
	workspaceMemberCount  metric.Int64Histogram
	searchRequests        metric.Int64Counter
	searchDuration        metric.Float64Histogram
	searchPhaseDuration   metric.Float64Histogram
	searchCandidate       metric.Int64Histogram
	searchHydrated        metric.Int64Histogram
	searchDropped         metric.Int64Histogram
	writerCommands        metric.Int64Counter
	writerChangeCount     metric.Int64Histogram
	projectionTransitions metric.Int64Counter
	projectionDuration    metric.Float64Histogram
	evidenceAppends       metric.Int64Counter
	evidenceDuration      metric.Float64Histogram
	telemetryDropped      metric.Int64Counter
	hookDispatches        metric.Int64Counter
	projectionLagging     atomic.Int64
	projectionPendingAt   atomic.Int64
	projectionProvider    atomic.Value
	startupErr            error
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
	} else if cfg.EnableOTLP && otlpEndpointConfigured() {
		if exportErr := validateOTLPEndpoint(); exportErr != nil {
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
	r := &Runtime{
		registry: registry, metricProvider: mp, traceProvider: tp,
		tracer: tp.Tracer(ScopeName), propagator: propagation.TraceContext{}, startupErr: startupErr,
	}
	r.projectionProvider.Store("other")
	meter := mp.Meter(ScopeName)
	if r.httpDuration, err = meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10)); err != nil {
		return nil, err
	}
	if r.httpActive, err = meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}")); err != nil {
		return nil, err
	}
	if r.opExecutions, err = meter.Int64Counter("kc.operation.executions", metric.WithUnit("{operation}")); err != nil {
		return nil, err
	}
	if r.opDuration, err = meter.Float64Histogram("kc.operation.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10)); err != nil {
		return nil, err
	}
	if r.opActive, err = meter.Int64UpDownCounter("kc.operation.active", metric.WithUnit("{operation}")); err != nil {
		return nil, err
	}
	if r.authDecisions, err = meter.Int64Counter("kc.authorization.decisions", metric.WithUnit("{decision}")); err != nil {
		return nil, err
	}
	if r.workspaceDuration, err = meter.Float64Histogram("kc.workspace.resolve.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10)); err != nil {
		return nil, err
	}
	if r.workspaceMemberCount, err = meter.Int64Histogram("kc.workspace.member.count", metric.WithUnit("{repository}"), metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 25, 50, 100)); err != nil {
		return nil, err
	}
	if r.searchRequests, err = meter.Int64Counter("kc.search.requests", metric.WithUnit("{request}")); err != nil {
		return nil, err
	}
	if r.searchDuration, err = meter.Float64Histogram("kc.search.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10)); err != nil {
		return nil, err
	}
	if r.searchPhaseDuration, err = meter.Float64Histogram("kc.search.phase.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.0001, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10)); err != nil {
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
	if r.writerChangeCount, err = meter.Int64Histogram("kc.writer.change.count", metric.WithUnit("{change}"), metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 25, 50, 100, 250, 500, 1000)); err != nil {
		return nil, err
	}
	if r.projectionTransitions, err = meter.Int64Counter("kc.projection.transitions", metric.WithUnit("{transition}")); err != nil {
		return nil, err
	}
	if r.projectionDuration, err = meter.Float64Histogram("kc.projection.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 300)); err != nil {
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
	if startupErr != nil {
		r.telemetryDropped.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("kc.telemetry.signal", "trace"),
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
	if metricErr != nil {
		return metricErr
	}
	return traceErr
}

func (r *Runtime) ForceFlush(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if err := r.metricProvider.ForceFlush(ctx); err != nil {
		return err
	}
	return r.traceProvider.ForceFlush(ctx)
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

func (r *Runtime) RecordAuthorization(ctx context.Context, operation, decision string) {
	r.authDecisions.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kc.operation", bounded(operation, "other")),
		attribute.String("kc.authorization.decision", enumValue(decision, "deny", "allow", "deny")),
	))
}

func (r *Runtime) RecordWorkspaceResolve(ctx context.Context, outcome string, elapsed time.Duration, members int) {
	outcomeAttr := attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "partial", "unresolved", "denied", "invalid", "conflict", "error"))
	r.workspaceDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(outcomeAttr))
	if members >= 0 {
		r.workspaceMemberCount.Record(ctx, int64(members), metric.WithAttributes(outcomeAttr))
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

func (r *Runtime) RecordWriter(ctx context.Context, surface, outcome, errorType string, replayed bool, puts, removes int) {
	attrs := []attribute.KeyValue{
		attribute.String("kc.writer.surface", enumValue(surface, "other", "COMMIT", "PROPOSAL", "other")),
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "partial", "unresolved", "denied", "invalid", "conflict", "error")),
		attribute.Bool("kc.writer.replayed", replayed),
	}
	if errorType != "" && errorType != "none" {
		attrs = append(attrs, attribute.String("error.type", bounded(errorType, "other")))
	}
	r.writerCommands.Add(ctx, 1, metric.WithAttributes(attrs...))
	changeAttrs := []attribute.KeyValue{attrs[0]}
	if puts > 0 {
		r.writerChangeCount.Record(ctx, int64(puts), metric.WithAttributes(append(changeAttrs, attribute.String("kc.writer.change.operation", "PUT"))...))
	}
	if removes > 0 {
		r.writerChangeCount.Record(ctx, int64(removes), metric.WithAttributes(append(changeAttrs, attribute.String("kc.writer.change.operation", "REMOVE"))...))
	}
}

func (r *Runtime) RecordProjection(ctx context.Context, provider, mode, outcome string, elapsed time.Duration) {
	providerAttr := attribute.String("kc.retrieval.provider", enumValue(provider, "other", "none", "opensearch", "other"))
	r.projectionDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(
		providerAttr,
		attribute.String("kc.projection.mode", enumValue(mode, "other", "incremental", "rebuild", "ready", "other")),
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "partial", "unresolved", "denied", "invalid", "conflict", "error")),
	))
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
}

func (r *Runtime) RecordHook(ctx context.Context, phase, transport, outcome string) {
	r.hookDispatches.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kc.hook.phase", enumValue(phase, "other", "pre", "post", "other")),
		attribute.String("kc.hook.transport", enumValue(transport, "other", "exec", "http", "outbox", "other")),
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "error")),
	))
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

func otlpEndpointConfigured() bool {
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")) != "" ||
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != ""
}

func validateOTLPEndpoint() error {
	raw := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"))
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
