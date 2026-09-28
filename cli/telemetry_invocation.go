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
		if FlagString(flags, "request-id") == "" {
			flags["request-id"] = telemetry.NewID("req")
		}
		spanContext := operationStarted.Span.SpanContext()
		if FlagString(flags, "trace-id") == "" && spanContext.IsValid() {
			flags["trace-id"] = spanContext.TraceID().String()
		}
		if FlagString(flags, "span-id") == "" && spanContext.IsValid() {
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

// invokeApplicationWithTelemetryAtHome is the typed-service application
// boundary. Unlike the CLI dispatcher it receives an explicit operation and
// therefore never consults the CLI command registry.
func invokeApplicationWithTelemetryAtHome(ctx context.Context, runtime *telemetry.Runtime, name, action string, op command, flags map[string]FlagValue, state knowledgeserving.StateLookup, opened *Home) RunResult {
	if flags == nil {
		flags = map[string]FlagValue{}
	}
	flags["_action"] = action
	operationStarted := telemetryStart{}
	observation := &operationTelemetry{}
	operationEnded := false
	if runtime != nil {
		transportSpanID := FlagString(flags, "span-id")
		remoteParentSpanID := FlagString(flags, "parent-span-id")
		ctx, operationStarted.Span, operationStarted.At = runtime.StartOperation(ctx, telemetryFace(name), name)
		observation = newOperationTelemetry(ctx, runtime, name, func() string { return telemetryProvider(flags) })
		defer func() {
			if !operationEnded {
				runtime.EndOperation(ctx, operationStarted.Span, operationStarted.At, telemetryFace(name), name, "error", "other")
			}
		}()
		spanContext := operationStarted.Span.SpanContext()
		// W3C HTTP requests already carry the SERVER span coordinates in flags.
		// Point application evidence at this child span while preserving legacy
		// correlation tokens that cannot be represented as an OTel parent.
		if spanContext.IsValid() && (FlagString(flags, "trace-id") == "" || FlagString(flags, "trace-id") == spanContext.TraceID().String()) {
			flags["trace-id"] = spanContext.TraceID().String()
			flags["span-id"] = spanContext.SpanID().String()
			if remoteParentSpanID != "" {
				// Evidence keeps the caller's W3C parent coordinate so incoming
				// trace context can be reconciled independently of KC's internal
				// SERVER/application span layering.
				flags["parent-span-id"] = remoteParentSpanID
			} else if transportSpanID != "" && transportSpanID != spanContext.SpanID().String() {
				flags["parent-span-id"] = transportSpanID
			}
		}
	}
	var invokeErr error
	if _, err := requestIDFrom(flags); err != nil {
		invokeErr = err
	}
	if invokeErr == nil {
		if _, err := identityContextFrom(flags); err != nil {
			invokeErr = err
		}
	}
	if invokeErr == nil {
		if _, err := traceContextFrom(flags); err != nil {
			invokeErr = err
		}
	}
	if invokeErr == nil {
		invokeErr = rejectUnknownFlags(flags)
	}
	home := ""
	if invokeErr == nil {
		home, invokeErr = resolveHome(flags)
	}
	var result any
	executed := false
	if invokeErr == nil {
		result, invokeErr = executeApplicationOperation(ctx, name, action, op, flags, state, opened, home, observation)
		executed = true
	}
	domainElapsed := telemetrySince(operationStarted.At)
	if executed {
		result, invokeErr = evidenceChain(ctx, runtime, home, name, flags, result, invokeErr)
	}
	if runtime != nil {
		recordDomainTelemetry(ctx, runtime, name, flags, observation, result, invokeErr, domainElapsed)
		outcome, errorType := telemetryResultFor(name, result, invokeErr)
		runtime.EndOperation(ctx, operationStarted.Span, operationStarted.At, telemetryFace(name), name, outcome, errorType)
		operationEnded = true
	}
	return shapeInvocationResult(result, invokeErr)
}
