package telemetry

// Instrument registration for the process metric contract. Names, units and
// bucket boundaries are the observable contract; keeping them in one flat list
// makes drift reviewable and keeps runtime.go's control flow readable.

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// registerInstruments creates every KC instrument on one meter. It is a flat
// registration list kept out of New so that constructor control flow stays
// reviewable; the names, units and buckets are the metric contract.
func (r *Runtime) registerInstruments(meter metric.Meter) error {
	var err error
	if r.httpDuration, err = meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return err
	}
	if r.httpActive, err = meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}")); err != nil {
		return err
	}
	if r.opExecutions, err = meter.Int64Counter("kc.operation.executions", metric.WithUnit("{operation}")); err != nil {
		return err
	}
	if r.opDuration, err = meter.Float64Histogram("kc.operation.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return err
	}
	if r.opActive, err = meter.Int64UpDownCounter("kc.operation.active", metric.WithUnit("{operation}")); err != nil {
		return err
	}
	if r.authenticationAttempts, err = meter.Int64Counter("kc.authentication.attempts", metric.WithUnit("{attempt}")); err != nil {
		return err
	}
	if r.authenticationDuration, err = meter.Float64Histogram("kc.authentication.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return err
	}
	if r.authDecisions, err = meter.Int64Counter("kc.authorization.decisions", metric.WithUnit("{decision}")); err != nil {
		return err
	}
	if r.identityRequests, err = meter.Int64Counter("kc.identity.requests", metric.WithUnit("{request}")); err != nil {
		return err
	}
	if r.workspaceDuration, err = meter.Float64Histogram("kc.workspace.resolve.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return err
	}
	if r.workspaceMemberCount, err = meter.Int64Histogram("kc.workspace.member.count", metric.WithUnit("{repository}"), metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 25, 50, 100)); err != nil {
		return err
	}
	if r.searchRequests, err = meter.Int64Counter("kc.search.requests", metric.WithUnit("{request}")); err != nil {
		return err
	}
	if r.searchDuration, err = meter.Float64Histogram("kc.search.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return err
	}
	if r.searchPhaseDuration, err = meter.Float64Histogram("kc.search.phase.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(searchPhaseDurationSecondsBuckets...)); err != nil {
		return err
	}
	if r.searchCandidate, err = meter.Int64Histogram("kc.search.candidate.count", metric.WithUnit("{candidate}"), metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 20, 50, 100, 250, 500, 1000)); err != nil {
		return err
	}
	if r.searchHydrated, err = meter.Int64Histogram("kc.search.hydrated.count", metric.WithUnit("{object}"), metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 20, 50, 100, 250, 500, 1000)); err != nil {
		return err
	}
	if r.searchDropped, err = meter.Int64Histogram("kc.search.dropped.count", metric.WithUnit("{candidate}"), metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 20, 50, 100, 250, 500, 1000)); err != nil {
		return err
	}
	if r.writerCommands, err = meter.Int64Counter("kc.writer.commands", metric.WithUnit("{command}")); err != nil {
		return err
	}
	if r.writerDuration, err = meter.Float64Histogram("kc.writer.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return err
	}
	if r.writerChangeCount, err = meter.Int64Histogram("kc.writer.change.count", metric.WithUnit("{change}"), metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 25, 50, 100, 250, 500, 1000)); err != nil {
		return err
	}
	if r.writerPayloadSize, err = meter.Int64Histogram("kc.writer.payload.size", metric.WithUnit("By"), metric.WithExplicitBucketBoundaries(byteBuckets...)); err != nil {
		return err
	}
	if r.projectionTransitions, err = meter.Int64Counter("kc.projection.transitions", metric.WithUnit("{transition}")); err != nil {
		return err
	}
	if r.projectionDuration, err = meter.Float64Histogram("kc.projection.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 300)); err != nil {
		return err
	}
	if r.projectionChangeCount, err = meter.Int64Histogram("kc.projection.change.count", metric.WithUnit("{document}"), metric.WithExplicitBucketBoundaries(countBuckets...)); err != nil {
		return err
	}
	if _, err = meter.Int64ObservableGauge("kc.projection.lagging.count", metric.WithUnit("{projection}"), metric.WithInt64Callback(func(_ context.Context, observer metric.Int64Observer) error {
		provider, _ := r.projectionProvider.Load().(string)
		observer.Observe(r.projectionLagging.Load(), metric.WithAttributes(attribute.String("kc.retrieval.provider", provider)))
		return nil
	})); err != nil {
		return err
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
		return err
	}
	if _, err = meter.Int64ObservableGauge("kc.projection.documents", metric.WithUnit("{document}"), metric.WithInt64Callback(func(_ context.Context, observer metric.Int64Observer) error {
		provider, _ := r.projectionProvider.Load().(string)
		observer.Observe(r.projectionDocuments.Load(), metric.WithAttributes(attribute.String("kc.retrieval.provider", provider)))
		return nil
	})); err != nil {
		return err
	}
	if r.bindingLookups, err = meter.Int64Counter("kc.binding.lookups", metric.WithUnit("{lookup}")); err != nil {
		return err
	}
	if r.bindingDuration, err = meter.Float64Histogram("kc.binding.lookup.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return err
	}
	if r.bindingObservationAge, err = meter.Float64Histogram("kc.binding.observation.age", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(observationAgeSecondsBuckets...)); err != nil {
		return err
	}
	if r.evidenceAppends, err = meter.Int64Counter("kc.evidence.appends", metric.WithUnit("{append}")); err != nil {
		return err
	}
	if r.evidenceDuration, err = meter.Float64Histogram("kc.evidence.append.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.0001, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1)); err != nil {
		return err
	}
	if r.telemetryDropped, err = meter.Int64Counter("kc.telemetry.dropped", metric.WithUnit("{record}")); err != nil {
		return err
	}
	if r.hookDispatches, err = meter.Int64Counter("kc.hook.dispatches", metric.WithUnit("{dispatch}")); err != nil {
		return err
	}
	if r.hookDuration, err = meter.Float64Histogram("kc.hook.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(requestDurationSecondsBuckets...)); err != nil {
		return err
	}
	if _, err = meter.Int64ObservableGauge("kc.hook.outbox.pending", metric.WithUnit("{event}"), metric.WithInt64Callback(func(_ context.Context, observer metric.Int64Observer) error {
		observer.Observe(r.hookOutboxPending.Load())
		return nil
	})); err != nil {
		return err
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
		return err
	}
	if r.gateChecks, err = meter.Int64Counter("kc.gate.checks", metric.WithUnit("{check}")); err != nil {
		return err
	}
	if r.gateDuration, err = meter.Float64Histogram("kc.gate.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(searchPhaseDurationSecondsBuckets...)); err != nil {
		return err
	}
	if r.vfsTransferSize, err = meter.Int64Histogram("kc.vfs.transfer.size", metric.WithUnit("By"), metric.WithExplicitBucketBoundaries(byteBuckets...)); err != nil {
		return err
	}
	if r.vfsDirectoryEntries, err = meter.Int64Histogram("kc.vfs.directory.entry.count", metric.WithUnit("{entry}"), metric.WithExplicitBucketBoundaries(countBuckets...)); err != nil {
		return err
	}
	return nil
}
