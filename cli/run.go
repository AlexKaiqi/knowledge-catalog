package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kc/hook"
	"kc/internal/telemetry"
	"kc/kernel"
	knowledgeserving "kc/knowledge/serving"
	"kc/snapshot"
)

// Entry point and result shaping. Public grouped commands are resolved by
// surface.go into internal application operations.

const (
	// defaultRef is snapshot.DefaultRef, re-exported for local readability.
	defaultRef = snapshot.DefaultRef

	defaultHome    = ".kc"
	ownerPrincipal = "owner"

	defaultAuditLimit = 50
	// unboundedLimit is what --limit 0 means: no cap.
	unboundedLimit = 1 << 30
)

type RunResult struct {
	Status int
	Stdout string
}

// Run is the argv entry. `serve` is handled here because it does not return a
// result; every other command resolves through the grouped CLI surface.
func Run(argv []string) RunResult {
	return RunWithTelemetry(argv, nil)
}

// RunWithTelemetry is the process entry used by cmd/kc. Library callers can
// keep using Run/Invoke without installing an SDK runtime.
func RunWithTelemetry(argv []string, runtime *telemetry.Runtime) RunResult {
	parsed, err := ParseArgs(argv)
	if err != nil {
		return errorResult(err)
	}
	if parsed.Command == "serve" {
		if len(parsed.Args) != 0 {
			return errorResult(fmt.Errorf("unexpected argument %s", parsed.Args[0]))
		}
		if FlagBool(parsed.Flags, "help") {
			return RunResult{Status: 0, Stdout: Help + "\n"}
		}
		return runServe(parsed.Flags)
	}
	if parsed.Command == "help" || parsed.Command == "--help" || parsed.Command == "-h" {
		if len(parsed.Args) > 0 {
			parsed.Flags["topic"] = strings.Join(parsed.Args, " ")
		}
		return invokeWithTelemetry(context.Background(), runtime, "help", parsed.Flags)
	}
	surface, positionals, err := resolveCLICommand(parsed.Command, parsed.Args)
	if err != nil {
		return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "%v", err))
	}
	if err := applyPositionals(surface.Handler, parsed.Flags, positionals); err != nil {
		return errorResult(err)
	}
	publicPath := strings.Join(append([]string{parsed.Command}, parsed.Args[:len(parsed.Args)-len(positionals)]...), " ")
	if publicPath == "catalog workspace resolve" && (FlagString(parsed.Flags, "object") != "" || FlagString(parsed.Flags, "aspect") != "" || FlagString(parsed.Flags, "member") != "") {
		return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "catalog workspace resolve returns only a fixed Workspace pin; use kc knowledge read for an object"))
	}
	if err := inheritTaskContext(publicPath, parsed.Flags); err != nil {
		return errorResult(err)
	}
	parsed.Flags["_action"] = surface.Action
	if strings.HasPrefix(publicPath, "local ") {
		if strings.TrimSpace(FlagString(parsed.Flags, "server")) != "" {
			return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "kc local commands cannot use --server"))
		}
		return invokeWithTelemetry(context.Background(), runtime, surface.Handler, parsed.Flags)
	}
	if server := remoteServerURL(parsed.Flags); server != "" {
		return runRemoteCLI(context.Background(), server, publicPath, parsed.Flags)
	}
	return invokeWithTelemetry(context.Background(), runtime, surface.Handler, parsed.Flags)
}

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
	operationEnded := false
	if runtime != nil {
		ctx, operationStarted.span, operationStarted.at = runtime.StartOperation(ctx, telemetryFace(command), command)
		flags["_telemetry-authorization-observer"] = authorizationObserver(func(decision string) {
			runtime.RecordAuthorization(ctx, command, decision)
		})
		flags["_telemetry-hook-observer"] = hook.DispatchObserver(func(phase, transport, outcome string) {
			runtime.RecordHook(ctx, phase, transport, outcome)
		})
		flags["_telemetry-projection-backlog-observer"] = projectionBacklogObserver(func(lagging int, oldestPendingAt time.Time) {
			runtime.SetProjectionBacklog(telemetryProvider(flags), lagging, oldestPendingAt)
		})
		flags["_telemetry-evidence-observer"] = evidenceTelemetryObserver(func(kind, outcome string, elapsed time.Duration) {
			runtime.RecordEvidence(ctx, kind, outcome, elapsed)
		})
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
	result, err := dispatchWithStateAtHome(ctx, command, flags, state, home)
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
		recordDomainTelemetry(ctx, runtime, command, flags, result, err, domainElapsed)
		outcome, errorType := telemetryResultFor(command, result, err)
		runtime.EndOperation(ctx, operationStarted.span, operationStarted.at, telemetryFace(command), command, outcome, errorType)
		operationEnded = true
	}
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
	operationEnded := false
	if runtime != nil {
		parentSpanID := FlagString(flags, "span-id")
		ctx, operationStarted.span, operationStarted.at = runtime.StartOperation(ctx, telemetryFace(name), name)
		flags["_telemetry-authorization-observer"] = authorizationObserver(func(decision string) {
			runtime.RecordAuthorization(ctx, name, decision)
		})
		flags["_telemetry-hook-observer"] = hook.DispatchObserver(func(phase, transport, outcome string) {
			runtime.RecordHook(ctx, phase, transport, outcome)
		})
		flags["_telemetry-projection-backlog-observer"] = projectionBacklogObserver(func(lagging int, oldestPendingAt time.Time) {
			runtime.SetProjectionBacklog(telemetryProvider(flags), lagging, oldestPendingAt)
		})
		flags["_telemetry-evidence-observer"] = evidenceTelemetryObserver(func(kind, outcome string, elapsed time.Duration) {
			runtime.RecordEvidence(ctx, kind, outcome, elapsed)
		})
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
		result, invokeErr = executeApplicationOperation(ctx, name, action, op, flags, state, opened, home)
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
		recordDomainTelemetry(ctx, runtime, name, flags, result, invokeErr, domainElapsed)
		outcome, errorType := telemetryResultFor(name, result, invokeErr)
		runtime.EndOperation(ctx, operationStarted.span, operationStarted.at, telemetryFace(name), name, outcome, errorType)
		operationEnded = true
	}
	if invokeErr != nil {
		return errorResult(invokeErr)
	}
	if resultOutcome(result) == "partial" {
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

func resultOutcome(result any) string {
	if row, ok := jsonValue(accessOutput(result)).(map[string]any); ok {
		return strings.ToLower(stringValue(row["outcome"]))
	}
	return ""
}

func applyPositionals(command string, flags map[string]FlagValue, args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch command {
	case "help", "--help", "-h":
		if len(args) > 1 {
			return fmt.Errorf("unexpected argument %s", args[1])
		}
		if FlagString(flags, "topic") != "" {
			return fmt.Errorf("unexpected argument %s", args[0])
		}
		flags["topic"] = args[0]
		return nil
	case "repo-add":
		if len(args) > 1 {
			return fmt.Errorf("unexpected argument %s", args[1])
		}
		if FlagString(flags, "repo") != "" {
			return fmt.Errorf("unexpected argument %s", args[0])
		}
		flags["repo"] = args[0]
		return nil
	default:
		return fmt.Errorf("unexpected argument %s", args[0])
	}
}

func resolveHome(flags map[string]FlagValue) (string, error) {
	home := FlagString(flags, "home")
	if home == "" {
		home = strings.TrimSpace(os.Getenv("KC_HOME"))
		if home == "" {
			home = defaultHome
		}
	}
	return filepath.Abs(home)
}

func errorResult(err error) RunResult {
	return RunResult{Status: 1, Stdout: jsonOut(kernel.FaultJSON(err))}
}

func jsonOut(value any) string {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("{\n  \"error\": {\n    \"message\": %q\n  }\n}\n", err.Error())
	}
	return string(b) + "\n"
}
