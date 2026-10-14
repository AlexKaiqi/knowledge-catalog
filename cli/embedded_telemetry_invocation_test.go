package cli

import (
	"context"
	apphome "kc/home"

	"kc/internal/telemetry"
	inv "kc/invocation"
	knowledgeserving "kc/liveaccess"

	"kc/invoke"
)

// Invocation telemetry is the explicit application aspect. It owns trace,
// metric, and evidence weaving while commands retain only source facts.

// invokeInternal is used by package-local application assembly and tests. It
// is not a transport API; HTTP handlers call typed services instead.
func invokeInternal(command string, flags map[string]FlagValue) RunResult {
	return invokeWithTelemetry(context.Background(), nil, command, flags)
}

func invokeWithTelemetry(ctx context.Context, runtime *telemetry.Runtime, command string, flags map[string]FlagValue) RunResult {
	return invokeWithTelemetryAndState(ctx, runtime, command, flags, nil)
}

func invokeWithTelemetryAndState(ctx context.Context, runtime *telemetry.Runtime, command string, flags map[string]FlagValue, state knowledgeserving.StateLookup) RunResult {
	return invokeWithTelemetryAndStateAtHome(ctx, runtime, command, flags, state, nil)
}

func invokeWithTelemetryAndStateAtHome(ctx context.Context, runtime *telemetry.Runtime, command string, flags map[string]FlagValue, state knowledgeserving.StateLookup, home *apphome.Home) RunResult {
	if flags == nil {
		flags = map[string]FlagValue{}
	}
	operationStarted := inv.TelemetryStart{}
	observation := &invoke.ObservationTelemetry{}
	operationEnded := false
	if runtime != nil {
		ctx, operationStarted.Span, operationStarted.At = runtime.StartOperation(ctx, inv.TelemetryFace(command), command)
		observation = invoke.NewObservationTelemetry(ctx, runtime, command, func() string { return inv.TelemetryProvider(flags) })
		defer func() {
			if !operationEnded {
				runtime.EndOperation(ctx, operationStarted.Span, operationStarted.At, inv.TelemetryFace(command), command, "error", "other")
			}
		}()
		if invoke.FlagString(flags, "request-id") == "" {
			flags["request-id"] = telemetry.NewID("req")
		}
		spanContext := operationStarted.Span.SpanContext()
		if invoke.FlagString(flags, "trace-id") == "" && spanContext.IsValid() {
			flags["trace-id"] = spanContext.TraceID().String()
		}
		if invoke.FlagString(flags, "span-id") == "" && spanContext.IsValid() {
			flags["span-id"] = spanContext.SpanID().String()
		}
	}
	result, err := dispatchWithStateAtHome(ctx, command, flags, state, home, observation)
	domainElapsed := inv.TelemetrySince(operationStarted.At)
	if home, homeErr := resolveHome(flags); homeErr == nil {
		result, err = inv.EvidenceChain(ctx, runtime, home, command, flags, result, err)
	}
	if runtime != nil {
		inv.RecordDomainTelemetry(ctx, runtime, command, flags, observation, result, err, domainElapsed)
		outcome, errorType := inv.TelemetryResultFor(command, result, err)
		runtime.EndOperation(ctx, operationStarted.Span, operationStarted.At, inv.TelemetryFace(command), command, outcome, errorType)
		operationEnded = true
	}
	return inv.ShapeInvocationResult(result, err)
}
