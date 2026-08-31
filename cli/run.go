package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kc/internal/telemetry"
	"kc/kernel"
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
