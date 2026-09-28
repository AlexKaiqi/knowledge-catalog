package invocation

// The dispatchers are the seam's reason to exist: one resolved application
// operation, authorized, hook-wrapped, observed, and evidenced — identical for
// the argv front end (which resolves a grouped command first) and the typed
// service (which passes an explicit Command and never consults the CLI
// registry). API-01 lives here: both transports run the same knowledgeapp
// executors through the same stage machinery. (CAP-ORG-01 B5c.)

import (
	"context"
	"strings"

	kchome "kc/home"
	"kc/internal/telemetry"
	"kc/invoke"
	knowledgeserving "kc/knowledge/serving"
)

// executeApplicationOperation is the shared application-service boundary.
// CLI resolves a grouped command before calling it; typed HTTP handlers pass
// an explicit command value and never consult either CLI registry.
func ExecuteApplicationOperation(ctx context.Context, name, action string, cmd Command, flags map[string]invoke.FlagValue, state knowledgeserving.StateLookup, opened *kchome.Home, home string, observation *invoke.ObservationTelemetry) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	observation = invoke.NoObservationTelemetry(observation)
	cx := &Invocation{Command: name, Home: home, WS: opened, Flags: flags, Context: ctx, State: state, Observation: observation}
	if opened != nil && len(opened.File.Catalogs) > 0 && invoke.FlagString(flags, "catalog") == "" {
		flags["_default-catalog"] = opened.File.Catalogs[0].ID
	}
	if cmd.Stage == invoke.StageHome {
		if observesCatalogInventory(name) {
			if err := AuthorizeCatalogInventory(home, flags, observation.Authorization, opened); err != nil {
				return nil, err
			}
			return cmd.Run(cx)
		}
		if err := Authorize(home, action, flags, observation.Authorization, opened); err != nil {
			return nil, err
		}
		return cmd.Run(cx)
	}
	ws := opened
	if ws == nil {
		var err error
		ws, err = kchome.Open(home)
		if err != nil {
			return nil, err
		}
		defer ws.Close()
	}
	cx.WS = ws
	BindHomeTelemetry(observation.Runtime, ws, flags)
	// Production serving has no implicit recipe adoption. Its read operations
	// can borrow authority/index handles while keeping request stamps and the
	// accepted Catalog state private. Component fixtures retain their legacy
	// adoption path and therefore cannot use this immutable request view.
	if opened != nil && opened.Deployment != nil && TypedInvocationReadOnly(action) {
		requestJournal, err := RequestHomeJournal(ws.Dir, action, flags)
		if err != nil {
			return nil, err
		}
		cx.WS = ws.ReadView(requestJournal)
		if invoke.FlagString(flags, "repo") == "" {
			if err := PrepareKnowledgePinContext(flags); err != nil {
				return nil, err
			}
		}
		if err := Authorize(home, action, AuthorizationFlags(cx), observation.Authorization, ws); err != nil {
			return nil, err
		}
		return WithHooks(cx.WS, home, action, flags, observation, func() (any, error) { return cmd.Run(cx) })
	}
	if cmd.Stage == invoke.StageOpen {
		if err := Authorize(home, action, flags, observation.Authorization, ws); err != nil {
			return nil, err
		}
		ObserveHome(ws, action, flags)
		return cmd.Run(cx)
	}
	// Bind the Catalog-scoped control state before authorization so verbs such
	// as merge can derive their real repository/ref scope from the immutable
	// proposal instead of making callers repeat (and potentially spoof) it.
	if invoke.FlagString(flags, "repo") == "" {
		if err := PrepareKnowledgePinContext(flags); err != nil {
			return nil, err
		}
	}
	ws.BindControl(cx.Flag("catalog"))
	authorizationFlags := AuthorizationFlags(cx)
	if err := Authorize(home, action, authorizationFlags, observation.Authorization, ws); err != nil {
		return nil, err
	}
	ObserveHome(ws, action, flags)
	return WithHooks(ws, home, action, flags, observation, func() (any, error) {
		return cmd.Run(cx)
	})
}

func observesCatalogInventory(name string) bool {
	return name == "catalog-list" || name == "operations-stores"
}

// invokeApplicationWithTelemetryAtHome is the typed-service application
// boundary. Unlike the CLI dispatcher it receives an explicit operation and
// therefore never consults the CLI command registry.
func InvokeApplicationWithTelemetryAtHome(ctx context.Context, runtime *telemetry.Runtime, name, action string, op Command, flags map[string]invoke.FlagValue, state knowledgeserving.StateLookup, opened *kchome.Home) invoke.RunResult {
	if flags == nil {
		flags = map[string]invoke.FlagValue{}
	}
	flags["_action"] = action
	operationStarted := TelemetryStart{}
	observation := &invoke.ObservationTelemetry{}
	operationEnded := false
	if runtime != nil {
		transportSpanID := invoke.FlagString(flags, "span-id")
		remoteParentSpanID := invoke.FlagString(flags, "parent-span-id")
		ctx, operationStarted.Span, operationStarted.At = runtime.StartOperation(ctx, TelemetryFace(name), name)
		observation = invoke.NewObservationTelemetry(ctx, runtime, name, func() string { return TelemetryProvider(flags) })
		defer func() {
			if !operationEnded {
				runtime.EndOperation(ctx, operationStarted.Span, operationStarted.At, TelemetryFace(name), name, "error", "other")
			}
		}()
		spanContext := operationStarted.Span.SpanContext()
		// W3C HTTP requests already carry the SERVER span coordinates in flags.
		// Point application evidence at this child span while preserving legacy
		// correlation tokens that cannot be represented as an OTel parent.
		if spanContext.IsValid() && (invoke.FlagString(flags, "trace-id") == "" || invoke.FlagString(flags, "trace-id") == spanContext.TraceID().String()) {
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
	if _, err := RequestIDFrom(flags); err != nil {
		invokeErr = err
	}
	if invokeErr == nil {
		if _, err := IdentityContextFrom(flags); err != nil {
			invokeErr = err
		}
	}
	if invokeErr == nil {
		if _, err := TraceContextFrom(flags); err != nil {
			invokeErr = err
		}
	}
	if invokeErr == nil {
		invokeErr = RejectUnknownFlags(flags)
	}
	home := ""
	if invokeErr == nil {
		home, invokeErr = ResolveHome(flags)
	}
	var result any
	executed := false
	if invokeErr == nil {
		result, invokeErr = ExecuteApplicationOperation(ctx, name, action, op, flags, state, opened, home, observation)
		executed = true
	}
	domainElapsed := TelemetrySince(operationStarted.At)
	if executed {
		result, invokeErr = EvidenceChain(ctx, runtime, home, name, flags, result, invokeErr)
	}
	if runtime != nil {
		RecordDomainTelemetry(ctx, runtime, name, flags, observation, result, invokeErr, domainElapsed)
		outcome, errorType := TelemetryResultFor(name, result, invokeErr)
		runtime.EndOperation(ctx, operationStarted.Span, operationStarted.At, TelemetryFace(name), name, outcome, errorType)
		operationEnded = true
	}
	return ShapeInvocationResult(result, invokeErr)
}

// typedInvocationReadOnly classifies the actions that may run on the
// immutable per-request view. It is also the deployment contract for
// executeApplicationOperation's ReadView fast path.
func TypedInvocationReadOnly(action string) bool {
	if strings.HasSuffix(action, ".read") {
		return true
	}
	switch action {
	case "knowledge.search", "knowledge.rerank", "knowledge.relations", "knowledge.provenance",
		"knowledge.binding.resolve", "knowledge.access.describe", "resource.access", "dataset.resolve":
		return true
	default:
		return false
	}
}
