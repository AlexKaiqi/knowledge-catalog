package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"kc/internal/telemetry"
	"kc/kernel"
	knowledgeserving "kc/knowledge/serving"
	"kc/snapshot"
)

// Entry point and result shaping. Everything about a specific verb lives in the
// matching verbs_*.go file; the routing table is in command.go.

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
// result; every other verb goes through Invoke.
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
	if err := applyPositionals(parsed.Command, parsed.Flags, parsed.Args); err != nil {
		return errorResult(err)
	}
	if parsed.Command == "serve" {
		if FlagBool(parsed.Flags, "help") {
			return RunResult{Status: 0, Stdout: Help + "\n"}
		}
		return runServe(parsed.Flags)
	}
	return invokeWithTelemetry(context.Background(), runtime, parsed.Command, parsed.Flags)
}

// Invoke is the CLI/HTTP shared entry: one verb plus a flag map keyed by the
// same names as --flags. Both transports get the same audit record and the same
// {error:{code,message}} envelope.
func Invoke(command string, flags map[string]FlagValue) RunResult {
	return invokeWithTelemetry(context.Background(), nil, command, flags)
}

func invokeWithTelemetry(ctx context.Context, runtime *telemetry.Runtime, command string, flags map[string]FlagValue) RunResult {
	return invokeWithTelemetryAndState(ctx, runtime, command, flags, nil)
}

func invokeWithTelemetryAndState(ctx context.Context, runtime *telemetry.Runtime, command string, flags map[string]FlagValue, state knowledgeserving.StateLookup) RunResult {
	if flags == nil {
		flags = map[string]FlagValue{}
	}
	operationStarted := telemetryStart{}
	if runtime != nil {
		ctx, operationStarted.span, operationStarted.at = runtime.StartOperation(ctx, telemetryFace(command), command)
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
	result, err := dispatchWithState(ctx, command, flags, state)
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
		if auditErr := recordAudit(home, command, flags, result, err); auditErr != nil && err == nil {
			err = auditErr
			result = nil
		}
	}
	if runtime != nil {
		recordDomainTelemetry(ctx, runtime, command, flags, result, err, telemetrySince(operationStarted.at))
		outcome, errorType := telemetryResultFor(command, result, err)
		runtime.EndOperation(ctx, operationStarted.span, operationStarted.at, telemetryFace(command), command, outcome, errorType)
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
		home = defaultHome
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
