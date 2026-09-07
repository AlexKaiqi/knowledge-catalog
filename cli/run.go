package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
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

	defaultAuditLimit   = 50
	maxAuditPageSize    = 200
	defaultHistoryLimit = 50
	maxHistoryPageSize  = 200
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
	return runWithTelemetryMode(argv, runtime, false)
}

// runWithTelemetryMode keeps the embedded application path available to the
// package's conformance tests without exposing it as a product transport. A
// real CLI invocation is a deployment operation, client preprocessing, a
// server process, or a typed client of KC Server.
func runWithTelemetryMode(argv []string, runtime *telemetry.Runtime, allowEmbedded bool) RunResult {
	parsed, err := ParseArgs(argv)
	if err != nil {
		return errorResult(err)
	}
	if parsed.Command == "help" || FlagBool(parsed.Flags, "help") || parsed.Command == "--help" || parsed.Command == "-h" {
		topic := strings.Join(parsed.Args, " ")
		if parsed.Command != "help" && parsed.Command != "--help" && parsed.Command != "-h" {
			topic = strings.TrimSpace(parsed.Command + " " + topic)
		}
		help, err := helpFor(topic)
		if err != nil {
			return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "%v", err))
		}
		return RunResult{Stdout: help}
	}
	if parsed.Command == "serve" {
		if len(parsed.Args) != 0 {
			return errorResult(fmt.Errorf("unexpected argument %s", parsed.Args[0]))
		}
		return runServe(parsed.Flags)
	}

	surface, positionals, err := resolveCLICommand(parsed.Command, parsed.Args)
	if err != nil {
		return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "%v", err))
	}
	if err := applyPositionals(surface.Handler, parsed.Flags, positionals); err != nil {
		return errorResult(err)
	}
	publicPath := strings.Join(append([]string{parsed.Command}, parsed.Args[:len(parsed.Args)-len(positionals)]...), " ")
	if err := rejectPublicSurfaceFlags(publicPath, parsed.Flags); err != nil {
		return errorResult(err)
	}
	if publicPath == "workspace pin" && (FlagString(parsed.Flags, "object") != "" || FlagString(parsed.Flags, "aspect") != "" || FlagString(parsed.Flags, "member") != "") {
		return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "workspace pin returns only a fixed Workspace pin; use kc knowledge resolve for an object"))
	}
	if (publicPath == "knowledge log" || publicPath == "knowledge provenance") && usesAddress(parsed.Flags) {
		if publicPath == "knowledge log" {
			return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "knowledge log is object history; do not pass --aspect or --member"))
		}
		return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "knowledge provenance is object-level; do not pass --aspect or --member"))
	}
	parsed.Flags["_action"] = surface.Action
	if strings.HasPrefix(publicPath, "deployment ") || publicPath == "workspace overlay" || publicPath == "pack" {
		flags := parsed.Flags
		if allowEmbedded {
			flags = maps.Clone(flags)
			delete(flags, "home")
		}
		return runClientOperationWithTelemetry(context.Background(), runtime, publicPath, surface.Handler, flags)
	}
	if err := rejectUnknownFlags(parsed.Flags); err != nil {
		return errorResult(err)
	}
	if err := rejectServeOnlyFlags(parsed.Flags); err != nil {
		return errorResult(err)
	}
	if _, ok := parsed.Flags["config"]; ok {
		return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "--config is only valid for deployment operations and kc serve"))
	}
	if publicPath == "catalog repo attach" {
		for _, name := range []string{"dir", "driver", "dsn"} {
			if _, ok := parsed.Flags[name]; ok {
				return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "catalog repo attach uses a configured Snapshot binding; --%s belongs in deployment configuration", name))
			}
		}
	}
	if err := inheritTaskContext(publicPath, parsed.Flags); err != nil {
		return errorResult(err)
	}
	if server := remoteServerURL(parsed.Flags); server != "" {
		return runRemoteCLI(context.Background(), server, publicPath, parsed.Flags)
	}
	if !allowEmbedded {
		return errorResult(kernel.Fail(kernel.ErrUsageInvalid,
			"%s requires KC Server; set --server or KC_SERVER_URL (use kc deployment init --config for deployment setup)", publicPath))
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
	assign := func(flag string) error {
		if len(args) > 1 {
			return fmt.Errorf("unexpected argument %s", args[1])
		}
		if FlagString(flags, flag) != "" {
			return fmt.Errorf("unexpected argument %s", args[0])
		}
		flags[flag] = args[0]
		return nil
	}
	switch command {
	case "help", "--help", "-h":
		return assign("topic")
	case "catalog-repo-attach":
		return assign("repo")
	case "catalog-show", "catalog-archive", "catalog-audit":
		return assign("catalog")
	case "workspace-show", "workspace-pin", "workspace-check", "workspace-retire", "workspace-define":
		return assign("workspace")
	default:
		return fmt.Errorf("unexpected argument %s", args[0])
	}
}

func rejectPublicSurfaceFlags(publicPath string, flags map[string]FlagValue) error {
	if strings.HasPrefix(publicPath, "knowledge ") {
		if err := rejectMixedKnowledgeBasis(flags); err != nil {
			return err
		}
	}
	switch publicPath {
	case "catalog repo create":
		return validateManagedRepositoryCreateFlags(flags)
	case "knowledge access":
		if FlagString(flags, "operation") != "" || FlagString(flags, "input") != "" {
			return kernel.Fail(kernel.ErrUsageInvalid,
				"knowledge access hydrates a Binding with --aspect; use kc knowledge invoke for a ResourceDescriptor operation")
		}
	case "knowledge invoke":
		if FlagString(flags, "aspect") != "" || FlagString(flags, "member") != "" {
			return kernel.Fail(kernel.ErrUsageInvalid,
				"knowledge invoke calls a ResourceDescriptor operation; use kc knowledge access --aspect for Binding hydration")
		}
		if FlagString(flags, "operation") == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "knowledge invoke requires --operation and --input")
		}
	}
	return nil
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
