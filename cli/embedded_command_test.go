package cli

import (
	"context"
	apphome "kc/home"

	"kc/kernel"
	objectserving "kc/liveaccess"
)

import (
	inv "kc/invocation"
	"kc/invoke"

	"os"
	"path/filepath"
	"strings"
)

// commands is an internal application-operation registry. Public CLI paths are
// declared separately in surface.go; HTTP routes never read this table.
var commands = func() map[string]inv.Command {
	all := map[string]inv.Command{}
	for _, group := range []map[string]inv.Command{
		homeVerbs(),
		loginVerbs(),
		observabilityVerbs(),
		allowVerbs(),
		policyVerbs(),
		writeVerbs(),
		readVerbs(),
		maintenanceVerbs(),
		resourceVerbs(),
		indexVerbs(),
		datasetVerbs(),
		controlVerbs(),
	} {
		for name, cmd := range group {
			if _, dup := all[name]; dup {
				panic("duplicate kc verb: " + name)
			}
			all[name] = cmd
		}
	}
	return all
}()

// operation reports whether name is an internal application operation.
func operation(name string) bool {
	_, ok := commands[name]
	return ok
}

// dispatchWithStateAtHome lets embedded component tests reuse an already-opened
// Home across requests. Embedded CLI fixtures pass nil for a command-scoped
// lifecycle.
func dispatchWithStateAtHome(ctx context.Context, name string, flags map[string]FlagValue, state objectserving.StateLookup, opened *apphome.Home, observation *invoke.ObservationTelemetry) (any, error) {
	home, err := resolveHome(flags)
	if err != nil {
		return nil, err
	}
	if _, err := inv.RequestIDFrom(flags); err != nil {
		return nil, err
	}
	if _, err := inv.IdentityContextFrom(flags); err != nil {
		return nil, err
	}
	if _, err := inv.TraceContextFrom(flags); err != nil {
		return nil, err
	}
	if isHelp(name, flags) {
		return helpFor(invoke.FlagString(flags, "topic"))
	}
	if err := rejectRemovedFlags(flags); err != nil {
		return nil, err
	}
	cmd, ok := commands[name]
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown command %s", name)
	}
	if err := inv.RejectUnknownFlags(flags); err != nil {
		return nil, err
	}
	if err := inv.RejectServeOnlyFlags(flags); err != nil {
		return nil, err
	}
	action := inv.ActionOf(name, flags)
	return inv.ExecuteApplicationOperation(ctx, name, action, cmd, flags, state, opened, home, observation)
}

func isHelp(name string, flags map[string]FlagValue) bool {
	switch name {
	case "help", "--help", "-h":
		return true
	}
	return invoke.FlagBool(flags, "help")
}

func resolveHome(flags map[string]FlagValue) (string, error) {
	home := invoke.FlagString(flags, "home")
	if home == "" {
		home = strings.TrimSpace(os.Getenv("KC_HOME"))
		if home == "" {
			home = inv.DefaultHome
		}
	}
	return filepath.Abs(home)
}
