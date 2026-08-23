package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"kc/kernel"
	"kc/repository"
)

// Entry point and result shaping. Everything about a specific verb lives in the
// matching verbs_*.go file; the routing table is in command.go.

const (
	// defaultRef is repository.DefaultRef, re-exported for local readability.
	defaultRef = repository.DefaultRef

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
	parsed, err := ParseArgs(argv)
	if err != nil {
		return errorResult(err)
	}
	if err := applyPositionals(parsed.Command, parsed.Flags, parsed.Args); err != nil {
		return errorResult(err)
	}
	if parsed.Command == "serve" {
		return runServe(parsed.Flags)
	}
	return Invoke(parsed.Command, parsed.Flags)
}

// Invoke is the CLI/HTTP shared entry: one verb plus a flag map keyed by the
// same names as --flags. Both transports get the same audit record and the same
// {error:{code,message}} envelope.
func Invoke(command string, flags map[string]FlagValue) RunResult {
	if flags == nil {
		flags = map[string]FlagValue{}
	}
	result, err := dispatch(command, flags)
	if home, homeErr := resolveHome(flags); homeErr == nil {
		if auditErr := recordAudit(home, command, flags, result, err); auditErr != nil && err == nil {
			err = auditErr
			result = nil
		}
	}
	if err != nil {
		return errorResult(err)
	}
	if text, ok := result.(string); ok {
		if strings.HasSuffix(text, "\n") {
			return RunResult{Status: 0, Stdout: text}
		}
		return RunResult{Status: 0, Stdout: text + "\n"}
	}
	return RunResult{Status: 0, Stdout: jsonOut(result)}
}

func applyPositionals(command string, flags map[string]FlagValue, args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch command {
	case "mount", "repo-add":
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
