package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"kc/internal/clientconfig"
	"kc/internal/telemetry"
	inv "kc/invocation"
	"kc/invoke"
	"kc/kernel"
)

// Entry point and result shaping. Public grouped commands are resolved by
// surface.go into internal application operations.

// RunResult is the shared invocation result; the shape lives in kc/invoke so
// the CLI front and the embedded server can both depend on it without a cycle.
type RunResult = invoke.RunResult

// Run is the argv entry. `serve` is handled here because it does not return a
// result; every other command resolves through the grouped CLI surface.
func Run(argv []string) RunResult {
	return RunWithTelemetry(argv, nil)
}

// RunWithTelemetry is the process entry used by cmd/kc. Business commands use
// the authenticated Server transport; deployment and host tasks stay local.
func RunWithTelemetry(argv []string, runtime *telemetry.Runtime) RunResult {
	return decorateCLIUsage(argv, dispatchCLI(argv, runtime))
}

type cliRequest struct {
	path    string
	surface commandSurface
	flags   map[string]FlagValue
}

func dispatchCLI(argv []string, runtime *telemetry.Runtime) RunResult {
	request, completed := prepareCLI(argv)
	if completed != nil {
		return *completed
	}
	if request.path == "serve" {
		return runServe(request.flags)
	}
	if err := validateCLIRequest(request); err != nil {
		return inv.ErrorResult(err)
	}
	if clientOperationPath(request.path) {
		return runClientOperationWithTelemetry(context.Background(), runtime, request.path, request.surface.Handler, request.flags)
	}
	if server := clientconfig.ServerURL(invoke.FlagString(request.flags, "server")); server != "" {
		return encodeCLIProduct(request.path, request.flags, runRemoteCLI(context.Background(), server, request.path, request.flags))
	}
	return inv.ErrorResult(kernel.Fail(kernel.ErrUsageInvalid,
		"%s requires KC Server; obtain an existing service address and set --server or KC_SERVER_URL; see kc help login (deployment owners: kc help deployment)", request.path))
}

// prepareCLI owns the common argv contract. Execution is selected only after
// this preparation, so a fixture can reuse parsing without entering production.
func prepareCLI(argv []string) (cliRequest, *RunResult) {
	stop := func(result RunResult) (cliRequest, *RunResult) {
		return cliRequest{}, &result
	}
	parsed, err := ParseArgs(argv)
	if err != nil {
		return stop(inv.ErrorResult(err))
	}
	if parsed.Command == "help" || invoke.FlagBool(parsed.Flags, "help") || parsed.Command == "--help" || parsed.Command == "-h" {
		topic := strings.Join(parsed.Args, " ")
		if parsed.Command != "help" && parsed.Command != "--help" && parsed.Command != "-h" {
			topic = strings.TrimSpace(parsed.Command + " " + topic)
		}
		help, err := helpFor(topic)
		if err != nil {
			return stop(inv.ErrorResult(kernel.Fail(kernel.ErrUsageInvalid, "%v", err)))
		}
		return stop(RunResult{Stdout: help})
	}
	if parsed.Command == "serve" {
		if len(parsed.Args) != 0 {
			return stop(inv.ErrorResult(fmt.Errorf("unexpected argument %s", parsed.Args[0])))
		}
		return cliRequest{path: "serve", flags: parsed.Flags}, nil
	}

	surface, positionals, err := resolveCLICommand(parsed.Command, parsed.Args)
	if err != nil {
		parts := append([]string{parsed.Command}, parsed.Args...)
		if topic, ok := incompletePublicFamily(parts); ok {
			help, helpErr := helpFor(topic)
			if helpErr != nil {
				return stop(inv.ErrorResult(kernel.Fail(kernel.ErrUsageInvalid, "%v", err)))
			}
			return stop(RunResult{Status: 1, Stdout: help})
		}
		return stop(inv.ErrorResult(kernel.Fail(kernel.ErrUsageInvalid, "%v", err)))
	}
	if err := applyPositionals(surface.Handler, parsed.Flags, positionals); err != nil {
		return stop(inv.ErrorResult(err))
	}
	publicPath := strings.Join(append([]string{parsed.Command}, parsed.Args[:len(parsed.Args)-len(positionals)]...), " ")
	return cliRequest{path: publicPath, surface: surface, flags: parsed.Flags}, nil
}

func clientOperationPath(path string) bool {
	return strings.HasPrefix(path, "deployment ") || path == "dataset overlay"
}

func validateCLIRequest(request cliRequest) error {
	if err := rejectPublicSurfaceFlags(request.path, request.flags); err != nil {
		return err
	}
	request.flags["_action"] = request.surface.Action
	if clientOperationPath(request.path) {
		return nil
	}
	if err := rejectHostFlags(request.path, request.flags); err != nil {
		return err
	}
	if err := inv.RejectServeOnlyFlags(request.flags); err != nil {
		return err
	}
	if _, ok := request.flags["config"]; ok {
		return kernel.Fail(kernel.ErrUsageInvalid, "--config is only valid for deployment operations and kc serve")
	}
	return nil
}

func applyPositionals(command string, flags map[string]FlagValue, args []string) error {
	if len(args) == 0 {
		return nil
	}
	if command == "diff" {
		return bindObjectOperands(objectComparisonCLIEntry, flags, args)
	}
	if entry, module := objectCommandByHandler(command); module {
		return bindObjectOperands(entry, flags, args)
	}
	if entry, module := snapshotCommandByHandler(command); module {
		return bindSnapshotOperands(entry, flags, args)
	}
	if entry, module := discoveryCommandByHandler(command); module {
		return bindDiscoveryOperands(entry, flags, args)
	}
	if entry, module := declarativeIndexCommandByHandler(command); module {
		return bindDeclarativeIndexOperands(entry, flags, args)
	}
	if entry, module := liveAccessCommandByHandler(command); module {
		return bindModuleOperands(entry.Operands, flags, args)
	}
	assign := func(flag string) error {
		if len(args) > 1 {
			return fmt.Errorf("unexpected argument %s", args[1])
		}
		if invoke.FlagString(flags, flag) != "" {
			return fmt.Errorf("unexpected argument %s", args[0])
		}
		flags[flag] = args[0]
		return nil
	}
	switch command {
	case "help", "--help", "-h":
		return assign("topic")
	default:
		return fmt.Errorf("unexpected argument %s", args[0])
	}
}

func rejectPublicSurfaceFlags(publicPath string, flags map[string]FlagValue) error {
	if publicPath == "diff" {
		if invoke.FlagString(flags, "reference") != "" {
			return validateObjectFlags(publicPath, flags)
		}
		for _, name := range objectComparisonCLIEntry.Flags {
			if _, supplied := flags[name]; supplied {
				return kernel.Fail(kernel.ErrUsageInvalid, "kc diff --%s requires an object URN as its positional argument", name)
			}
		}
	}
	if _, explicitCatalog := flags["catalog"]; explicitCatalog {
		return kernel.Fail(kernel.ErrUsageInvalid, "%s rejects --catalog; one service has one registration space", publicPath)
	}
	if _, ok := flags["pin"]; ok {
		return kernel.Fail(kernel.ErrUsageInvalid, "%s rejects --pin; pass --dataset or --repo", publicPath)
	}
	if objectCommandPath(publicPath) {
		return validateObjectFlags(publicPath, flags)
	}
	if discoveryCommandPath(publicPath) {
		return validateDiscoveryFlags(publicPath, flags)
	}
	if declarativeIndexCommandPath(publicPath) {
		return validateDeclarativeIndexFlags(publicPath, flags)
	}
	if liveAccessCommandPath(publicPath) {
		return validateLiveAccessFlags(publicPath, flags)
	}
	if err := validateSnapshotFlags(publicPath, flags); err != nil {
		return err
	}
	if objectCLIPath(publicPath) || strings.HasPrefix(publicPath, "writer ") || publicPath == "diff" {
		if err := inv.RejectMixedObjectBasis(flags); err != nil {
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
	case "operations access-spec describe":
		if (invoke.FlagString(flags, "dataset") == "") == (invoke.FlagString(flags, "repo") == "") {
			return kernel.Fail(kernel.ErrUsageInvalid, "operations access-spec describe requires exactly one of --repo or --dataset")
		}
	}
	return nil
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

// Module-owned flags are checked by their single declaration contract.
func rejectHostFlags(path string, flags map[string]FlagValue) error {
	if objectCommandPath(path) || discoveryCommandPath(path) || declarativeIndexCommandPath(path) {
		return nil
	}
	return inv.RejectUnknownFlags(flags)
}
