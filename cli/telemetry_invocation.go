package cli

import (
	"context"

	"kc/internal/telemetry"
	knowledgeserving "kc/knowledge/serving"
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

func invokeWithTelemetryAndStateAtHome(ctx context.Context, runtime *telemetry.Runtime, command string, flags map[string]FlagValue, state knowledgeserving.StateLookup, home *Home) RunResult {
	if flags == nil {
		flags = map[string]FlagValue{}
	}
	operationStarted := telemetryStart{}
	observation := &operationTelemetry{}
	operationEnded := false
	if runtime != nil {
		ctx, operationStarted.Span, operationStarted.At = runtime.StartOperation(ctx, telemetryFace(command), command)
		observation = newOperationTelemetry(ctx, runtime, command, func() string { return telemetryProvider(flags) })
		defer func() {
			if !operationEnded {
				runtime.EndOperation(ctx, operationStarted.Span, operationStarted.At, telemetryFace(command), command, "error", "other")
			}
		}()
		if flagString(flags, "request-id") == "" {
			flags["request-id"] = telemetry.NewID("req")
		}
		spanContext := operationStarted.Span.SpanContext()
		if flagString(flags, "trace-id") == "" && spanContext.IsValid() {
			flags["trace-id"] = spanContext.TraceID().String()
		}
		if flagString(flags, "span-id") == "" && spanContext.IsValid() {
			flags["span-id"] = spanContext.SpanID().String()
		}
	}
	result, err := dispatchWithStateAtHome(ctx, command, flags, state, home, observation)
	domainElapsed := telemetrySince(operationStarted.At)
	if home, homeErr := resolveHome(flags); homeErr == nil {
		result, err = evidenceChain(ctx, runtime, home, command, flags, result, err)
	}
	if runtime != nil {
		recordDomainTelemetry(ctx, runtime, command, flags, observation, result, err, domainElapsed)
		outcome, errorType := telemetryResultFor(command, result, err)
		runtime.EndOperation(ctx, operationStarted.Span, operationStarted.At, telemetryFace(command), command, outcome, errorType)
		operationEnded = true
	}
	return shapeInvocationResult(result, err)
}
