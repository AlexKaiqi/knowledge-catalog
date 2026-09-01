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
		result, err = evidenceChain(ctx, runtime, home, command, flags, result, err)
	}
	if runtime != nil {
		recordDomainTelemetry(ctx, runtime, command, flags, observation, result, err, domainElapsed)
		outcome, errorType := telemetryResultFor(command, result, err)
		runtime.EndOperation(ctx, operationStarted.span, operationStarted.at, telemetryFace(command), command, outcome, errorType)
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
	domainElapsed := telemetrySince(operationStarted.at)
	if executed {
		result, invokeErr = evidenceChain(ctx, runtime, home, name, flags, result, invokeErr)
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

// evidenceChain is the single ordered evidence pipeline both public entry
// points owe: access, then retrieval, then refine, then audit. Keeping one
// implementation is what guarantees the CLI and the typed HTTP service record
// identical evidence for the same operation. A persistence failure becomes the
// caller's error so a response is never delivered without its evidence.
func evidenceChain(
	ctx context.Context,
	runtime *telemetry.Runtime,
	home, command string,
	flags map[string]FlagValue,
	result any,
	callErr error,
) (any, error) {
	accessStarted := telemetryNow()
	evidenceID, accessErr := recordKnowledgeAccess(home, command, flags, result, callErr)
	if runtime != nil && knowledgeAccessCommand(command, flags) {
		runtime.RecordEvidence(ctx, "access", telemetryOutcome(accessErr), telemetrySince(accessStarted))
	}
	if accessErr != nil && callErr == nil {
		callErr = accessErr
		result = nil
	}
	if evidenceID != "" {
		flags["_evidence-id"] = evidenceID
	}
	retrievalStarted := telemetryNow()
	retrievalID, retrievalErr := recordRetrievalEvidence(home, command, flags, result, evidenceID, callErr)
	if runtime != nil && (retrievalID != "" || retrievalErr != nil) {
		runtime.RecordEvidence(ctx, "retrieval", telemetryOutcome(retrievalErr), telemetrySince(retrievalStarted))
	}
	if retrievalErr != nil && callErr == nil {
		callErr = retrievalErr
	}
	if retrievalID != "" {
		flags["_retrieval-evidence-id"] = retrievalID
		result = attachRetrievalEvidenceID(result, retrievalID)
	}
	refineStarted := telemetryNow()
	refineID, refineErr := recordRefineEvidence(home, result, evidenceID, retrievalID)
	if runtime != nil && (refineID != "" || refineErr != nil) {
		runtime.RecordEvidence(ctx, "refine", telemetryOutcome(refineErr), telemetrySince(refineStarted))
	}
	if refineErr != nil && callErr == nil {
		callErr = refineErr
		result = nil
	}
	if refineID != "" {
		flags["_refine-evidence-id"] = refineID
		result = attachRefineEvidenceID(result, refineID)
	}
	result = accessOutput(result)
	auditStarted := telemetryNow()
	auditErr := recordAudit(home, command, flags, result, callErr)
	if runtime != nil && shouldAudit(command, flags) {
		runtime.RecordEvidence(ctx, "audit", telemetryOutcome(auditErr), telemetrySince(auditStarted))
	}
	if auditErr != nil && callErr == nil {
		callErr = auditErr
		result = nil
	}
	return result, callErr
}
