package cli

import (
	"context"
	"time"

	"kc/hook"
	"kc/internal/telemetry"
)

// operationTelemetry is one invocation's explicit observation dependency and
// source-fact accumulator. It is deliberately separate from FlagValue: flags
// are protocol inputs, while these callbacks and measurements are process-local.
type operationTelemetry struct {
	authorization     authorizationObserver
	hook              hook.DispatchObserver
	projection        projectionBacklogObserver
	evidence          evidenceTelemetryObserver
	putCount          int
	removeCount       int
	writerCountsSet   bool
	projectionElapsed time.Duration
}

func newOperationTelemetry(ctx context.Context, runtime *telemetry.Runtime, operation string, flags map[string]FlagValue) *operationTelemetry {
	observation := &operationTelemetry{}
	if runtime == nil {
		return observation
	}
	observation.authorization = func(decision string) {
		runtime.RecordAuthorization(ctx, operation, decision)
	}
	observation.hook = func(phase, transport, outcome string) {
		runtime.RecordHook(ctx, phase, transport, outcome)
	}
	observation.projection = func(lagging int, oldestPendingAt time.Time) {
		runtime.SetProjectionBacklog(telemetryProvider(flags), lagging, oldestPendingAt)
	}
	observation.evidence = func(kind, outcome string, elapsed time.Duration) {
		runtime.RecordEvidence(ctx, kind, outcome, elapsed)
	}
	return observation
}

func noOperationTelemetry(observation *operationTelemetry) *operationTelemetry {
	if observation == nil {
		return &operationTelemetry{}
	}
	return observation
}
