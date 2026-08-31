package cli

import (
	"context"
	"sort"
	"strings"

	"kc/internal/telemetry"
)

// RunEmbeddedForTest exercises the shared application service without making
// the removed public local-CLI transport available in production. Product
// client/server behavior is covered through Run and the typed HTTP routes.
func RunEmbeddedForTest(argv []string, runtime *telemetry.Runtime) RunResult {
	if result, ok := runRetiredMaintenanceForTest(argv, runtime); ok {
		return result
	}
	return runWithTelemetryMode(argv, runtime, true)
}

// retiredMaintenanceSurface keeps historical application-level tests useful
// after host-filesystem maintenance commands were removed from the product
// CLI. These operations are not included in CLICommandsForTest and cannot be
// resolved by the production process entry.
var retiredMaintenanceSurface = map[string]commandSurface{
	"maintenance object diff":        {"diff", "maintenance.object.diff"},
	"maintenance workspace inspect":  {"inspect", "maintenance.workspace.inspect"},
	"maintenance workspace checkout": {"checkout", "maintenance.workspace.checkout"},
	"maintenance workspace sync":     {"sync", "maintenance.workspace.checkout"},
	"maintenance workspace status":   {"status", "maintenance.workspace.inspect"},
	"maintenance snapshot export":    {"snapshot-export", "maintenance.snapshot.scan"},
}

func runRetiredMaintenanceForTest(argv []string, runtime *telemetry.Runtime) (RunResult, bool) {
	parsed, err := ParseArgs(argv)
	if err != nil || parsed.Command != "maintenance" {
		return RunResult{}, false
	}
	parts := append([]string{parsed.Command}, parsed.Args...)
	for n := len(parts); n > 0; n-- {
		path := strings.Join(parts[:n], " ")
		surface, ok := retiredMaintenanceSurface[path]
		if !ok {
			continue
		}
		if err := applyPositionals(surface.Handler, parsed.Flags, parts[n:]); err != nil {
			return errorResult(err), true
		}
		if err := inheritTaskContext(path, parsed.Flags); err != nil {
			return errorResult(err), true
		}
		parsed.Flags["_action"] = surface.Action
		return invokeWithTelemetry(context.Background(), runtime, surface.Handler, parsed.Flags), true
	}
	return RunResult{}, false
}

// CLICommandsForTest exposes the single CLI surface only inside the Go test
// binary. Coverage evidence does not justify expanding the production Go API.
func CLICommandsForTest() []string {
	commands := make([]string, 0, len(cliSurface))
	for path := range cliSurface {
		commands = append(commands, path)
	}
	sort.Strings(commands)
	return commands
}

func CLICommandForTest(path string) bool {
	_, ok := cliSurface[path]
	return ok
}

func CLICommandActionsForTest() map[string]string {
	actions := make(map[string]string, len(cliSurface))
	for path, surface := range cliSurface {
		actions[path] = surface.Action
	}
	return actions
}
