package cli

import (
	"context"
	"strings"

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
		ctx, operationStarted.span, operationStarted.at = runtime.StartOperation(ctx, telemetryFace(command), command)
		observation = newOperationTelemetry(ctx, runtime, command, flags)
		defer func() {
			if !operationEnded {
				runtime.EndOperation(ctx, operationStarted.span, operationStarted.at, telemetryFace(command), command, "error", "other")
			}
		}()
		if FlagString(flags, "request-id") == "" {
			flags["request-id"] = telemetry.NewID("req")
		}
		spanContext := operationStarted.span.SpanContext()
		if FlagString(flags, "trace-id") == "" && spanContext.IsValid() {
			flags["trace-id"] = spanContext.TraceID().String()
		}
		if FlagString(flags, "span-id") == "" && spanContext.IsValid() {
			flags["span-id"] = spanContext.SpanID().String()
		}
	}
	result, err := dispatchWithStateAtHome(ctx, command, flags, state, home, observation)
	domainElapsed := telemetrySince(operationStarted.at)
	if home, homeErr := resolveHome(flags); homeErr == nil {
		accessStarted := telemetryNow()
		evidenceID, accessErr := recordKnowledgeAccess(home, command, flags, result, err)
		if runtime != nil && knowledgeAccessCommand(command, flags) {
			runtime.RecordEvidence(ctx, "access", telemetryOutcome(accessErr), telemetrySince(accessStarted))
		}
		if accessErr != nil && err == nil {
			err = accessErr
			result = nil
		}
		if evidenceID != "" {
			flags["_evidence-id"] = evidenceID
		}
		result = accessOutput(result)
		auditStarted := telemetryNow()
		auditErr := recordAudit(home, command, flags, result, err)
		if runtime != nil && shouldAudit(command, flags) {
			runtime.RecordEvidence(ctx, "audit", telemetryOutcome(auditErr), telemetrySince(auditStarted))
		}
		if auditErr != nil && err == nil {
			err = auditErr
			result = nil
		}
	}
	if runtime != nil {
		recordDomainTelemetry(ctx, runtime, command, flags, observation, result, err, domainElapsed)
		outcome, errorType := telemetryResultFor(command, result, err)
		runtime.EndOperation(ctx, operationStarted.span, operationStarted.at, telemetryFace(command), command, outcome, errorType)
		operationEnded = true
	}
	return shapeInvocationResult(result, err)
}

// invokeApplicationAtHome is used only after a typed transport endpoint has
// selected one explicit application operation. It performs the same policy,
// evidence, and result shaping as local CLI without looking up a CLI path,
// parsing flags, or consulting the internal operation registry.
func invokeApplicationAtHome(ctx context.Context, name, action string, op command, flags map[string]FlagValue, state knowledgeserving.StateLookup, opened *Home) RunResult {
	return invokeApplicationWithTelemetryAtHome(ctx, nil, name, action, op, flags, state, opened)
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
		parentSpanID := FlagString(flags, "span-id")
		ctx, operationStarted.span, operationStarted.at = runtime.StartOperation(ctx, telemetryFace(name), name)
		observation = newOperationTelemetry(ctx, runtime, name, flags)
		defer func() {
			if !operationEnded {
				runtime.EndOperation(ctx, operationStarted.span, operationStarted.at, telemetryFace(name), name, "error", "other")
			}
		}()
		spanContext := operationStarted.span.SpanContext()
		// W3C HTTP requests already carry the SERVER span coordinates in flags.
		// Point application evidence at this child span while preserving legacy
		// correlation tokens that cannot be represented as an OTel parent.
		if spanContext.IsValid() && (FlagString(flags, "trace-id") == "" || FlagString(flags, "trace-id") == spanContext.TraceID().String()) {
			flags["trace-id"] = spanContext.TraceID().String()
			flags["span-id"] = spanContext.SpanID().String()
			if parentSpanID != "" && parentSpanID != spanContext.SpanID().String() {
				flags["parent-span-id"] = parentSpanID
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
	domainElapsed := telemetrySince(operationStarted.at)
	if executed {
		accessStarted := telemetryNow()
		evidenceID, accessErr := recordKnowledgeAccess(home, name, flags, result, invokeErr)
		if runtime != nil && knowledgeAccessCommand(name, flags) {
			runtime.RecordEvidence(ctx, "access", telemetryOutcome(accessErr), telemetrySince(accessStarted))
		}
		if accessErr != nil && invokeErr == nil {
			invokeErr = accessErr
			result = nil
		}
		if evidenceID != "" {
			flags["_evidence-id"] = evidenceID
		}
		result = accessOutput(result)
		auditStarted := telemetryNow()
		auditErr := recordAudit(home, name, flags, result, invokeErr)
		if runtime != nil && shouldAudit(name, flags) {
			runtime.RecordEvidence(ctx, "audit", telemetryOutcome(auditErr), telemetrySince(auditStarted))
		}
		if auditErr != nil && invokeErr == nil {
			invokeErr = auditErr
			result = nil
		}
	}
	if runtime != nil {
		recordDomainTelemetry(ctx, runtime, name, flags, observation, result, invokeErr, domainElapsed)
		outcome, errorType := telemetryResultFor(name, result, invokeErr)
		runtime.EndOperation(ctx, operationStarted.span, operationStarted.at, telemetryFace(name), name, outcome, errorType)
		operationEnded = true
	}
	return shapeInvocationResult(result, invokeErr)
}

func shapeInvocationResult(result any, err error) RunResult {
	if err != nil {
		return errorResult(err)
	}
	if resultOutcome(result) == "partial" {
		// Some repositories have already committed, so this is not an error
		// envelope and must retain every per-repository receipt. It is still a
		// non-zero CLI outcome so automation cannot mistake it for full success.
		return RunResult{Status: 2, Stdout: jsonOut(result)}
	}
	if text, ok := result.(string); ok {
		if strings.HasSuffix(text, "\n") {
			return RunResult{Status: 0, Stdout: text}
		}
		return RunResult{Status: 0, Stdout: text + "\n"}
	}
	return RunResult{Status: 0, Stdout: jsonOut(result)}
}
