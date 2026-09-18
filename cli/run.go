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
	return decorateCLIUsage(argv, dispatchCLI(argv, runtime, allowEmbedded))
}

func dispatchCLI(argv []string, runtime *telemetry.Runtime, allowEmbedded bool) RunResult {
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
		parts := append([]string{parsed.Command}, parsed.Args...)
		if topic, ok := incompletePublicFamily(parts); ok {
			help, helpErr := helpFor(topic)
			if helpErr != nil {
				return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "%v", err))
			}
			return RunResult{Status: 1, Stdout: help}
		}
		return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "%v", err))
	}
	if err := applyPositionals(surface.Handler, parsed.Flags, positionals); err != nil {
		return errorResult(err)
	}
	publicPath := strings.Join(append([]string{parsed.Command}, parsed.Args[:len(parsed.Args)-len(positionals)]...), " ")
	if err := rejectPublicSurfaceFlags(publicPath, parsed.Flags); err != nil {
		return errorResult(err)
	}
	if (publicPath == "log" || publicPath == "provenance") && usesAddress(parsed.Flags) {
		if publicPath == "log" {
			return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "log is object history; do not pass --aspect or --member"))
		}
		return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "provenance is object-level; do not pass --aspect or --member"))
	}
	parsed.Flags["_action"] = surface.Action
	if strings.HasPrefix(publicPath, "deployment ") || publicPath == "dataset overlay" {
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
	if _, explicitHome := parsed.Flags["home"]; allowEmbedded && explicitHome {
		if _, explicitServer := parsed.Flags["server"]; !explicitServer {
			// An explicitly embedded fixture owns its Home. Client defaults and
			// task context belong to the separate product transport; an explicit
			// --server still takes the normal mutually-exclusive validation path.
			return encodeCLIProduct(publicPath, parsed.Flags, invokeWithTelemetry(context.Background(), runtime, surface.Handler, parsed.Flags))
		}
	}
	if err := inheritTaskContext(publicPath, parsed.Flags); err != nil {
		return errorResult(err)
	}
	if server := remoteServerURL(parsed.Flags); server != "" {
		return encodeCLIProduct(publicPath, parsed.Flags, runRemoteCLI(context.Background(), server, publicPath, parsed.Flags))
	}
	if !allowEmbedded {
		return errorResult(kernel.Fail(kernel.ErrUsageInvalid,
			"%s requires KC Server; set --server or KC_SERVER_URL (use kc deployment init --config for deployment setup)", publicPath))
	}
	return encodeCLIProduct(publicPath, parsed.Flags, invokeWithTelemetry(context.Background(), runtime, surface.Handler, parsed.Flags))
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
	case "catalog-use":
		return assign("catalog")
	case "attach", "detach":
		return assign("repo")
	case "dataset-retire", "dataset-define":
		return assign("dataset")
	default:
		return fmt.Errorf("unexpected argument %s", args[0])
	}
}

func rejectPublicSurfaceFlags(publicPath string, flags map[string]FlagValue) error {
	if _, explicitCatalog := flags["catalog"]; explicitCatalog && publicPath != "catalog use" && publicPath != "grant add" && publicPath != "grant list" {
		return kernel.Fail(kernel.ErrUsageInvalid, "%s rejects --catalog; select the current Catalog with kc catalog use", publicPath)
	}
	if _, ok := flags["pin"]; ok {
		return kernel.Fail(kernel.ErrUsageInvalid, "%s rejects --pin; pass --dataset or --repo", publicPath)
	}
	if knowledgeCLIPath(publicPath) || strings.HasPrefix(publicPath, "writer ") || publicPath == "diff" {
		if err := rejectMixedKnowledgeBasis(flags); err != nil {
			return err
		}
		rejected := []string{"catalog", "source"}
		if publicPath == "schema list" || strings.HasPrefix(publicPath, "writer ") || publicPath == "diff" {
			rejected = append(rejected, "dataset")
		}
		for _, name := range rejected {
			if _, ok := flags[name]; ok {
				return kernel.Fail(kernel.ErrUsageInvalid, "%s rejects --%s; pass --dataset or --repo", publicPath, name)
			}
		}
	}
	switch publicPath {
	case "create":
		return validateManagedRepositoryCreateFlags(flags)
	case "governance preview create":
		for _, name := range []string{"catalog", "source"} {
			if _, ok := flags[name]; ok {
				return kernel.Fail(kernel.ErrUsageInvalid, "governance preview create rejects --%s; pass --dataset", name)
			}
		}
		if FlagString(flags, "dataset") == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "governance preview create requires --dataset")
		}
	case "operations access-spec describe":
		if (FlagString(flags, "dataset") == "") == (FlagString(flags, "repo") == "") {
			return kernel.Fail(kernel.ErrUsageInvalid, "operations access-spec describe requires exactly one of --repo or --dataset")
		}
	case "access":
		if FlagString(flags, "operation") != "" || FlagString(flags, "input") != "" {
			return kernel.Fail(kernel.ErrUsageInvalid,
				"access hydrates a Binding with --aspect; use kc invoke for a ResourceDescriptor operation")
		}
	case "invoke":
		if FlagString(flags, "aspect") != "" || FlagString(flags, "member") != "" {
			return kernel.Fail(kernel.ErrUsageInvalid,
				"invoke calls a ResourceDescriptor operation; use kc access --aspect for Binding hydration")
		}
		if FlagString(flags, "operation") == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "invoke requires --operation and --input")
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

// decorateCLIUsage turns a USAGE_INVALID FaultJSON into reason + leaf usage.
// HTTP writeInvoke still uses errorResult and keeps the envelope. Incomplete
// family prefixes already print group help and are left alone.
func decorateCLIUsage(argv []string, result RunResult) RunResult {
	if result.Status == 0 {
		return result
	}
	fault, ok := decodeLeadingFault(result.Stdout)
	if !ok || fmt.Sprint(fault["code"]) != string(kernel.ErrUsageInvalid) {
		return result
	}
	usage := usageForFailure(publicPathFromArgv(argv))
	if usage == "" || strings.Contains(result.Stdout, usage) {
		return result
	}
	result.Stdout = string(kernel.ErrUsageInvalid) + ": " + fmt.Sprint(fault["message"]) + "\n\n" + usage + "\n"
	return result
}

func decodeLeadingFault(stdout string) (map[string]any, bool) {
	stdout = strings.TrimSpace(stdout)
	if !strings.HasPrefix(stdout, "{") {
		return nil, false
	}
	var payload map[string]any
	if err := json.NewDecoder(strings.NewReader(stdout)).Decode(&payload); err != nil {
		return nil, false
	}
	fault, _ := payload["error"].(map[string]any)
	if fault == nil {
		return nil, false
	}
	return fault, true
}

func publicPathFromArgv(argv []string) string {
	parsed, err := ParseArgs(argv)
	if err != nil || parsed.Command == "" {
		return ""
	}
	if parsed.Command == "help" || parsed.Command == "--help" || parsed.Command == "-h" {
		return "help"
	}
	_, positionals, err := resolveCLICommand(parsed.Command, parsed.Args)
	if err != nil {
		parts := append([]string{parsed.Command}, parsed.Args...)
		if topic, ok := incompletePublicFamily(parts); ok {
			return topic
		}
		return parsed.Command
	}
	return strings.Join(append([]string{parsed.Command}, parsed.Args[:len(parsed.Args)-len(positionals)]...), " ")
}

func jsonOut(value any) string {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("{\n  \"error\": {\n    \"message\": %q\n  }\n}\n", err.Error())
	}
	return string(b) + "\n"
}
