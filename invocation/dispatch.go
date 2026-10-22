package invocation

// The dispatchers are the seam's reason to exist: one resolved application
// operation, authorized, hook-wrapped, observed, and evidenced — identical for
// the argv front end (which resolves a grouped command first) and the typed
// service (which passes an explicit Command and never consults the CLI
// registry). API-01 lives here: both transports run the same object/service and
// discovery/service executors through the same stage machinery. (CAP-ORG-01 B5c.)

import (
	"context"
	"strings"

	kchome "kc/home"
	"kc/internal/telemetry"
	"kc/invoke"
	"kc/kernel"
	objectserving "kc/liveaccess"
	objectmodel "kc/object"
)

// executeApplicationOperation is the shared application-service boundary.
// CLI resolves a grouped command before calling it; typed HTTP handlers pass
// an explicit command value and never consult either CLI registry.
func ExecuteApplicationOperation(ctx context.Context, name, action string, cmd Command, flags map[string]invoke.FlagValue, state objectserving.StateLookup, opened *kchome.Home, home string, observation *invoke.ObservationTelemetry) (any, error) {
	if _, supplied := flags["catalog"]; supplied {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "Catalog selection is not supported; this service has one registry")
	}
	if name == "object-diff" {
		return executeObjectComparison(ctx, action, flags, state, opened, home, observation)
	}
	if err := NormalizeReferenceFlags(flags); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if name == "object-log" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, objectmodel.HistoryScanRequestTimeout)
		defer cancel()
	}
	observation = invoke.NoObservationTelemetry(observation)
	cx := &Invocation{Command: name, Home: home, WS: opened, Flags: flags, Context: ctx, State: state, Observation: observation}
	if cmd.Stage == invoke.StageHome {
		if observesInventory(name) {
			if err := AuthorizeInventory(home, flags, observation.Authorization, opened); err != nil {
				return nil, err
			}
			return cmd.Run(cx)
		}
		var authErr error
		if action == "admin.grants.read" || action == "admin.grants.manage" {
			authErr = AuthorizeGrantAdministration(cx, action)
		} else {
			authErr = Authorize(home, action, flags, observation.Authorization, opened)
		}
		if authErr != nil {
			return nil, authErr
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
	if err := prepareObjectReadBasis(cx, action); err != nil {
		return nil, err
	}
	if err := prepareHistoryContinuation(cx, action); err != nil {
		return nil, err
	}
	if name == "object-log" {
		if err := bindHistoryReadContext(cx); err != nil {
			return nil, err
		}
		ws = cx.WS
	}
	if err := bindReferenceScope(cx, action); err != nil {
		return nil, err
	}
	if err := prepareReferenceAuthorization(cx, action); err != nil {
		return nil, err
	}
	BindHomeTelemetry(observation.Runtime, ws, flags)
	// Production serving has no implicit recipe adoption. Its read operations
	// can borrow authority/index handles while keeping request stamps and the
	// accepted Registry state private. Component fixtures retain their legacy
	// adoption path and therefore cannot use this immutable request view.
	if opened != nil && opened.Deployment != nil && TypedInvocationReadOnly(action) {
		requestJournal, err := RequestHomeJournal(ws.Dir, action, flags)
		if err != nil {
			return nil, err
		}
		cx.WS = ws.ReadView(requestJournal)
		if invoke.FlagString(flags, "repo") == "" {
			if err := PrepareObjectPinContext(flags); err != nil {
				return nil, err
			}
		}
		if err := Authorize(home, action, AuthorizationFlags(cx), observation.Authorization, ws); err != nil {
			return nil, err
		}
		return WithHooks(cx.WS, home, ActionOf(name, flags), flags, observation, func() (any, error) { return cmd.Run(cx) })
	}
	if cmd.Stage == invoke.StageOpen {
		if err := Authorize(home, action, AuthorizationFlags(cx), observation.Authorization, ws); err != nil {
			return nil, err
		}
		cx.WS = observeInvocationHome(ws, opened != nil, action, flags)
		return cmd.Run(cx)
	}
	// Bind the pin context before authorization so verbs such as receipt can
	// derive their real repository/ref scope from the immutable ledger entry
	// instead of making callers repeat (and potentially spoof) it.
	if invoke.FlagString(flags, "repo") == "" {
		if err := PrepareObjectPinContext(flags); err != nil {
			return nil, err
		}
	}
	AuthorizationFlags := AuthorizationFlags(cx)
	if err := Authorize(home, action, AuthorizationFlags, observation.Authorization, ws); err != nil {
		return nil, err
	}
	cx.WS = observeInvocationHome(ws, opened != nil, action, flags)
	return WithHooks(cx.WS, home, ActionOf(name, flags), flags, observation, func() (any, error) {
		return cmd.Run(cx)
	})
}

// observeInvocationHome stamps the Home this invocation runs on. A Home the
// caller handed in is shared by concurrent requests, so the stamp lives on a
// request view; a Home opened for this call alone can carry it directly.
func observeInvocationHome(ws *kchome.Home, shared bool, action string, flags map[string]invoke.FlagValue) *kchome.Home {
	if shared {
		return ObserveRequest(ws, action, flags)
	}
	ObserveHome(ws, action, flags)
	return ws
}

func observesInventory(name string) bool {
	return name == "operations-stores"
}

// invokeApplicationWithTelemetryAtHome is the typed-service application
// boundary. Unlike the local dispatcher it requires a verified Server
// principal and receives an explicit operation without consulting a registry.
func InvokeApplicationWithTelemetryAtHome(ctx context.Context, runtime *telemetry.Runtime, name, action string, op Command, flags map[string]invoke.FlagValue, state objectserving.StateLookup, opened *kchome.Home) invoke.RunResult {
	if strings.TrimSpace(invoke.FlagString(flags, "as")) == "" {
		return ErrorResult(kernel.Fail(kernel.ErrUnauthenticated, "a verified Server principal is required"))
	}
	flags["_action"] = action
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, captureErr := captureReadRequest(ctx, name, flags)
	if captureErr != nil {
		return ErrorResult(captureErr)
	}
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
	case "object.search", "object.rerank", "object.relations", "object.provenance",
		"object.binding.resolve", "object.access.describe", "resource.access", "dataset.resolve":
		return true
	default:
		return false
	}
}
