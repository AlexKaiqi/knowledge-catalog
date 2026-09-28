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

// executeApplicationOperation is the shared application-service boundary.
// CLI resolves a grouped command before calling it; typed HTTP handlers pass
// an explicit command value and never consult either CLI registry.
func executeApplicationOperation(ctx context.Context, name, action string, cmd command, flags map[string]FlagValue, state knowledgeserving.StateLookup, opened *Home, home string, observation *operationTelemetry) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	observation = noOperationTelemetry(observation)
	cx := &invocation{Command: name, Home: home, WS: opened, Flags: flags, Context: ctx, State: state, Observation: observation}
	if opened != nil && len(opened.File.Catalogs) > 0 && FlagString(flags, "catalog") == "" {
		flags["_default-catalog"] = opened.File.Catalogs[0].ID
	}
	if cmd.Stage == stageHome {
		if observesCatalogInventory(name) {
			if err := authorizeCatalogInventory(home, flags, observation.Authorization, opened); err != nil {
				return nil, err
			}
			return cmd.Run(cx)
		}
		if err := authorize(home, action, flags, observation.Authorization, opened); err != nil {
			return nil, err
		}
		return cmd.Run(cx)
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
	bindHomeTelemetry(observation.Runtime, ws, flags)
	// Production serving has no implicit recipe adoption. Its read operations
	// can borrow authority/index handles while keeping request stamps and the
	// accepted Catalog state private. Component fixtures retain their legacy
	// adoption path and therefore cannot use this immutable request view.
	if opened != nil && opened.Deployment != nil && typedInvocationReadOnly(action) {
		requestJournal, err := requestHomeJournal(ws.Dir, action, flags)
		if err != nil {
			return nil, err
		}
		cx.WS = ws.ReadView(requestJournal)
		if FlagString(flags, "repo") == "" {
			if err := prepareKnowledgePinContext(flags); err != nil {
				return nil, err
			}
		}
		if err := authorize(home, action, authorizationFlags(cx), observation.Authorization, ws); err != nil {
			return nil, err
		}
		return withHooks(cx.WS, home, action, flags, observation, func() (any, error) { return cmd.Run(cx) })
	}
	if cmd.Stage == stageOpen {
		if err := authorize(home, action, flags, observation.Authorization, ws); err != nil {
			return nil, err
		}
		observeHome(ws, action, flags)
		return cmd.Run(cx)
	}
	// Bind the Catalog-scoped control state before authorization so verbs such
	// as merge can derive their real repository/ref scope from the immutable
	// proposal instead of making callers repeat (and potentially spoof) it.
	if FlagString(flags, "repo") == "" {
		if err := prepareKnowledgePinContext(flags); err != nil {
			return nil, err
		}
	}
	ws.BindControl(cx.Flag("catalog"))
	authorizationFlags := authorizationFlags(cx)
	if err := authorize(home, action, authorizationFlags, observation.Authorization, ws); err != nil {
		return nil, err
	}
	observeHome(ws, action, flags)
	return withHooks(ws, home, action, flags, observation, func() (any, error) {
		return cmd.Run(cx)
	})
}

func observesCatalogInventory(name string) bool {
	return name == "catalog-list" || name == "operations-stores"
}

func isHelp(name string, flags map[string]FlagValue) bool {
	switch name {
	case "help", "--help", "-h":
		return true
	}
	return FlagBool(flags, "help")
}
