package telemetry

// Domain metric/trace emitters for the runtime. Each method owns one product
// concept (HTTP, snapshot, search, writer, projection, binding, evidence,
// hook, gate, VFS); names, units and buckets stay in instruments.go.

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

func (r *Runtime) RecordHTTP(ctx context.Context, started time.Time, method, route string, status int, propagationOutcome string) {
	attrs := []attribute.KeyValue{
		attribute.String("http.request.method", enumValue(method, "OTHER", "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "OTHER")),
		attribute.String("http.route", enumValue(route, "unmatched", "/", "/health", "/livez", "/readyz", "/readyz/{surface}", "/metrics",
			"/catalog/v1/{operation}", "/knowledge/v1/{operation}", "/dataset-files/v1/{operation}", "/writer/v1/{operation}",
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

func (r *Runtime) StartSnapshot(ctx context.Context, store, operation string) (context.Context, trace.Span, time.Time) {
	store = enumValue(store, "other", "lakefs", "gitea", "other")
	operation = enumValue(operation, "other", "resolve_ref", "read", "read_many", "list_page", "history", "diff", "commit", "compare_and_swap", "other")
	attrs := []attribute.KeyValue{
		attribute.String("kc.snapshot.store", store),
		attribute.String("kc.operation", operation),
	}
	r.snapshotActive.Add(ctx, 1, metric.WithAttributes(attrs...))
	ctx, span := r.tracer.Start(ctx, "kc.snapshot."+operation, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
	return ctx, span, time.Now()
}

func (r *Runtime) EndSnapshot(ctx context.Context, span trace.Span, started time.Time, store, operation, outcome, errorType string, bytes int64) {
	store = enumValue(store, "other", "lakefs", "gitea", "other")
	operation = enumValue(operation, "other", "resolve_ref", "read", "read_many", "list_page", "history", "diff", "commit", "compare_and_swap", "other")
	outcome = enumValue(outcome, "error", "ok", "partial", "unresolved", "denied", "invalid", "conflict", "error")
	base := []attribute.KeyValue{
		attribute.String("kc.snapshot.store", store),
		attribute.String("kc.operation", operation),
	}
	durationAttrs := append(append([]attribute.KeyValue{}, base...), attribute.String("kc.outcome", outcome))
	executionAttrs := append([]attribute.KeyValue{}, durationAttrs...)
	if errorType != "" && errorType != "none" {
		executionAttrs = append(executionAttrs, attribute.String("error.type", bounded(errorType, "other")))
	}
	r.snapshotActive.Add(ctx, -1, metric.WithAttributes(base...))
	r.snapshotOperations.Add(ctx, 1, metric.WithAttributes(executionAttrs...))
	r.snapshotDuration.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(durationAttrs...))
	if bytes >= 0 {
		r.snapshotBytes.Record(ctx, bytes, metric.WithAttributes(durationAttrs...))
		span.SetAttributes(attribute.Int64("kc.snapshot.operation.bytes", bytes))
	}
	span.SetAttributes(executionAttrs...)
	if outcome != "ok" && outcome != "partial" {
		span.SetStatus(codes.Error, bounded(errorType, "other"))
	}
	span.End()
}

func (r *Runtime) RecordKnowledgeReadFanout(ctx context.Context, objects, units int) {
	span := trace.SpanFromContext(ctx)
	if objects >= 0 {
		r.readObjectCount.Record(ctx, int64(objects))
		span.SetAttributes(attribute.Int("kc.read.object.count", objects))
	}
	if units >= 0 {
		r.readUnitCount.Record(ctx, int64(units))
		span.SetAttributes(attribute.Int("kc.read.unit.count", units))
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
	r.projectionBacklogSet.Store(true)
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

func (r *Runtime) RecordEvidence(ctx context.Context, kind, outcome string, elapsed time.Duration, bytes int64) {
	attrs := []attribute.KeyValue{
		attribute.String("kc.evidence.kind", enumValue(kind, "other", "access", "retrieval", "refine", "feedback", "system", "audit", "other")),
		attribute.String("kc.outcome", enumValue(outcome, "error", "ok", "partial", "unresolved", "denied", "invalid", "conflict", "error")),
	}
	r.evidenceAppends.Add(ctx, 1, metric.WithAttributes(attrs...))
	r.evidenceDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attrs...))
	if bytes >= 0 {
		r.evidenceBytes.Record(ctx, bytes, metric.WithAttributes(attrs...))
	}
	eventAttrs := append(attrs, attribute.Float64("kc.evidence.append.duration.seconds", elapsed.Seconds()))
	if bytes >= 0 {
		eventAttrs = append(eventAttrs, attribute.Int64("kc.evidence.append.bytes", bytes))
	}
	trace.SpanFromContext(ctx).AddEvent("kc.evidence.append", trace.WithAttributes(eventAttrs...))
}

func (r *Runtime) SetEvidenceStoreUsedRatio(ratio float64) {
	if ratio < 0 {
		r.evidenceUsedMilli.Store(-1)
		return
	}
	if ratio > 1 {
		ratio = 1
	}
	r.evidenceUsedMilli.Store(int64(ratio*10000 + 0.5))
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
