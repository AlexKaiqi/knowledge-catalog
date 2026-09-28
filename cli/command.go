package cli

import (
	"context"

	"kc/kernel"
	knowledgeserving "kc/knowledge/serving"
)

// commands is an internal application-operation registry. Public CLI paths are
// declared separately in surface.go; HTTP routes never read this table.
var commands = func() map[string]command {
	all := map[string]command{}
	for _, group := range []map[string]command{
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
		catalogVerbs(),
		connectionVerbs(),
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

// dispatchWithStateAtHome lets the HTTP service reuse one already-opened Home
// across read requests. CLI commands pass nil and retain their existing
// command-scoped lifecycle.
func dispatchWithStateAtHome(ctx context.Context, name string, flags map[string]FlagValue, state knowledgeserving.StateLookup, opened *Home, observation *operationTelemetry) (any, error) {
	home, err := resolveHome(flags)
	if err != nil {
		return nil, err
	}
	if _, err := requestIDFrom(flags); err != nil {
		return nil, err
	}
	if _, err := identityContextFrom(flags); err != nil {
		return nil, err
	}
	if _, err := traceContextFrom(flags); err != nil {
		return nil, err
	}
	if isHelp(name, flags) {
		return helpFor(FlagString(flags, "topic"))
	}
	if err := rejectRemovedFlags(flags); err != nil {
		return nil, err
	}
	cmd, ok := commands[name]
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown command %s", name)
	}
	if err := rejectUnknownFlags(flags); err != nil {
		return nil, err
	}
	if err := rejectServeOnlyFlags(flags); err != nil {
		return nil, err
	}
	action := actionOf(name, flags)
	return executeApplicationOperation(ctx, name, action, cmd, flags, state, opened, home, observation)
}

func isHelp(name string, flags map[string]FlagValue) bool {
	switch name {
	case "help", "--help", "-h":
		return true
	}
	return FlagBool(flags, "help")
}
