package cli

import (
	"context"

	"kc/kernel"
	knowledgeserving "kc/knowledge/serving"
)

// stage is how much of the local workspace a verb needs before it runs. It is
// the only axis on which verbs differ before their own body starts, so it is
// declared per verb instead of re-derived by an if-chain in the dispatcher.
type stage int

const (
	// stageHome resolves --home and nothing else. The verb opens what it needs,
	// which is how `audit` and `store-ls` still answer when Repository attachment fails.
	stageHome stage = iota

	// stageOpen opens this --home's object graph (stores, catalogs, writer)
	// and records the call. Used by the verbs that shape the home itself,
	// before any grant exists.
	stageOpen

	// stageGoverned mounts the workspace, checks allow.json, binds the control
	// plane and wraps the body in pre/post hooks. Every protocol verb is here.
	stageGoverned
)

// invocation is one resolved verb call. WS is nil at stageHome.
type invocation struct {
	Command     string
	Home        string
	Flags       map[string]FlagValue
	WS          *Home
	Context     context.Context
	State       knowledgeserving.StateLookup
	Observation *operationTelemetry
}

func (cx *invocation) flag(name string) string    { return FlagString(cx.Flags, name) }
func (cx *invocation) flags(name string) []string { return FlagStrings(cx.Flags, name) }
func (cx *invocation) require(name string) (string, error) {
	return RequireFlag(cx.Flags, name)
}

func (cx *invocation) workspaceID() (string, error) {
	return workspaceIDFlag(cx.Flags)
}

type handler func(cx *invocation) (any, error)

type command struct {
	stage stage
	run   handler
}

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

// executeApplicationOperation is the shared application-service boundary.
// CLI resolves a grouped command before calling it; typed HTTP handlers pass
// an explicit command value and never consult either CLI registry.
func executeApplicationOperation(ctx context.Context, name, action string, cmd command, flags map[string]FlagValue, state knowledgeserving.StateLookup, opened *Home, home string, observation *operationTelemetry) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	observation = noOperationTelemetry(observation)
	cx := &invocation{Command: name, Home: home, Flags: flags, Context: ctx, State: state, Observation: observation}
	if cmd.stage == stageHome {
		if err := authorize(home, action, flags, observation.authorization); err != nil {
			return nil, err
		}
		return cmd.run(cx)
	}
	ws := opened
	if ws == nil {
		var err error
		ws, err = Open(home)
		if err != nil {
			return nil, err
		}
		defer ws.Close()
	}
	cx.WS = ws
	if cmd.stage == stageOpen {
		if err := authorize(home, action, flags, observation.authorization); err != nil {
			return nil, err
		}
		ws.observe(action, flags)
		return cmd.run(cx)
	}
	// Bind the Catalog-scoped control state before authorization so verbs such
	// as merge can derive their real repository/ref scope from the immutable
	// proposal instead of making callers repeat (and potentially spoof) it.
	ws.bindControl(cx.flag("catalog"))
	authorizationFlags := authorizationFlags(cx)
	if err := authorize(home, action, authorizationFlags, observation.authorization); err != nil {
		return nil, err
	}
	ws.observe(action, flags)
	return withHooks(ws, home, action, flags, observation, func() (any, error) {
		return cmd.run(cx)
	})
}

func isHelp(name string, flags map[string]FlagValue) bool {
	switch name {
	case "help", "--help", "-h":
		return true
	}
	return FlagBool(flags, "help")
}
