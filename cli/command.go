package cli

import "kc/kernel"

// stage is how much of the local workspace a verb needs before it runs. It is
// the only axis on which verbs differ before their own body starts, so it is
// declared per verb instead of re-derived by an if-chain in the dispatcher.
type stage int

const (
	// stageHome resolves --home and nothing else. The verb opens what it needs,
	// which is how `audit` and `store-ls` still answer when mounting fails.
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
	Command string
	Home    string
	Flags   map[string]FlagValue
	WS      *Home
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

// commands is the single registry of kc verbs. Adding a verb means adding one
// entry plus one function in the matching verbs_*.go file; it does not mean
// editing a dispatcher. `kc serve` reads the same table, so HTTP and CLI can
// never drift on which verbs exist.
var commands = func() map[string]command {
	all := map[string]command{}
	for _, group := range []map[string]command{
		homeVerbs(),
		observabilityVerbs(),
		allowVerbs(),
		policyVerbs(),
		writeVerbs(),
		readVerbs(),
		vfsVerbs(),
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

// Verb reports whether name is a kc verb that Invoke can run.
func Verb(name string) bool {
	_, ok := commands[name]
	return ok
}

// dispatch resolves the verb, prepares its stage and runs it.
func dispatch(name string, flags map[string]FlagValue) (any, error) {
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
	cx := &invocation{Command: name, Home: home, Flags: flags}
	if cmd.stage == stageHome {
		return cmd.run(cx)
	}
	ws, err := Open(home)
	if err != nil {
		return nil, err
	}
	defer ws.Close()
	cx.WS = ws
	if cmd.stage == stageOpen {
		ws.observe(name, flags)
		return cmd.run(cx)
	}
	if err := authorize(home, consumerAllowCmd(name, flags), flags); err != nil {
		return nil, err
	}
	ws.observe(name, flags)
	ws.bindControl(cx.flag("catalog"))
	return withHooks(ws, home, name, flags, func() (any, error) {
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
