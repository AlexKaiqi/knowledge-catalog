package invoke

import (
	"context"
	"time"

	"kc/hook"
	"kc/internal/telemetry"
)

// AuthorizationObserver receives the authorization decision for one operation.
type AuthorizationObserver func(decision string)

// ProjectionBacklogObserver receives the projection backlog gauge.
type ProjectionBacklogObserver func(lagging int, oldestPendingAt time.Time)

// EvidenceTelemetryObserver receives one evidence write's outcome.
type EvidenceTelemetryObserver func(kind, outcome string, elapsed time.Duration, bytes int64)

// ObservationTelemetry is one invocation's explicit observation dependency and
// source-fact accumulator. It is deliberately separate from FlagValue: flags
// are protocol inputs, while these callbacks and measurements are
// process-local. (moved from cli/telemetry_operation.go, QUALITY-05 S3')
type ObservationTelemetry struct {
	Runtime            *telemetry.Runtime
	Authorization      AuthorizationObserver
	Hook               hook.DispatchObserver
	HookBacklog        func(pending int, oldestPendingAt time.Time)
	Projection         ProjectionBacklogObserver
	Evidence           EvidenceTelemetryObserver
	PutCount           int
	RemoveCount        int
	WriterPayloadBytes int
	WriterCountsSet    bool
	ProjectionElapsed  time.Duration

	// ctx is the request context the observers capture; it is part of the
	// observation, not of the transport request.
	ctx context.Context
}

// NewObservationTelemetry wires the runtime recorders for one operation. The
// projection backlog gauge needs the storage provider label, which is a
// front-end concern (it resolves --home and reads stores); the caller injects
// it as a closure so this package stays free of home resolution.
func NewObservationTelemetry(ctx context.Context, runtime *telemetry.Runtime, operation string, projectionProvider func() string) *ObservationTelemetry {
	observation := &ObservationTelemetry{Runtime: runtime, ctx: ctx}
	if runtime == nil {
		return observation
	}
	observation.Authorization = func(decision string) {
		runtime.RecordAuthorization(ctx, operation, decision)
	}
	observation.Hook = func(phase, transport, outcome string, elapsed time.Duration) {
		runtime.RecordHook(ctx, phase, transport, outcome, elapsed)
	}
	observation.HookBacklog = func(pending int, oldestPendingAt time.Time) {
		runtime.SetHookOutbox(pending, oldestPendingAt)
	}
	observation.Projection = func(lagging int, oldestPendingAt time.Time) {
		runtime.SetProjectionBacklog(projectionProvider(), lagging, oldestPendingAt)
	}
	observation.Evidence = func(kind, outcome string, elapsed time.Duration, bytes int64) {
		runtime.RecordEvidence(ctx, kind, outcome, elapsed, bytes)
	}
	return observation
}

// NoObservationTelemetry substitutes an inert observation so handlers never
// nil-check the accumulator.
func NoObservationTelemetry(observation *ObservationTelemetry) *ObservationTelemetry {
	if observation == nil {
		return &ObservationTelemetry{}
	}
	return observation
}
