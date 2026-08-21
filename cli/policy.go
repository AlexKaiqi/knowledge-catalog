package cli

import (
	"fmt"

	"kc/gate"
	"kc/hook"
	"kc/kernel"
)

func handleHook(home, command string, flags map[string]FlagValue) (any, error) {
	switch command {
	case "hook-add":
		on, err := RequireFlag(flags, "on")
		if err != nil {
			return nil, err
		}
		phase, err := RequireFlag(flags, "phase")
		if err != nil {
			return nil, err
		}
		binding := hook.Binding{
			On:      on,
			Phase:   phase,
			Repo:    FlagString(flags, "repo"),
			Catalog: FlagString(flags, "catalog"),
			Run:     FlagString(flags, "run"),
			URL:     FlagString(flags, "url"),
		}
		if err := hook.ValidateBinding(binding); err != nil {
			return nil, err
		}
		file, err := hook.Read(home)
		if err != nil {
			return nil, err
		}
		binding.ID = hook.NextID(file.Bindings)
		file.Bindings = append(file.Bindings, binding)
		if err := hook.Write(home, file); err != nil {
			return nil, err
		}
		return binding, nil
	case "hook-ls":
		file, err := hook.Read(home)
		if err != nil {
			return nil, err
		}
		on := FlagString(flags, "on")
		repo := FlagString(flags, "repo")
		catalogID := FlagString(flags, "catalog")
		out := []hook.Binding{}
		for _, b := range file.Bindings {
			if on != "" && b.On != on {
				continue
			}
			if repo != "" && b.Repo != repo {
				continue
			}
			if catalogID != "" && b.Catalog != catalogID {
				continue
			}
			out = append(out, b)
		}
		return map[string]any{"bindings": out}, nil
	case "hook-rm":
		id, err := RequireFlag(flags, "id")
		if err != nil {
			return nil, err
		}
		file, err := hook.Read(home)
		if err != nil {
			return nil, err
		}
		kept := file.Bindings[:0]
		found := false
		for _, b := range file.Bindings {
			if b.ID == id {
				found = true
				continue
			}
			kept = append(kept, b)
		}
		if !found {
			return nil, fmt.Errorf("unknown hook %s", id)
		}
		file.Bindings = kept
		if err := hook.Write(home, file); err != nil {
			return nil, err
		}
		return map[string]any{"revoked": id}, nil
	default:
		return nil, fmt.Errorf("unknown command %s", command)
	}
}

func handleGate(home, command string, flags map[string]FlagValue) (any, error) {
	switch command {
	case "gate-add":
		on, err := RequireFlag(flags, "on")
		if err != nil {
			return nil, err
		}
		if err := gate.ValidateOn(on); err != nil {
			return nil, err
		}
		require := gate.ParseRequire(FlagString(flags, "require"))
		if err := gate.ValidateRequire(require); err != nil {
			return nil, err
		}
		rule := gate.Rule{
			On:      on,
			Repo:    FlagString(flags, "repo"),
			Catalog: FlagString(flags, "catalog"),
			Release: FlagString(flags, "release"),
			Require: require,
		}
		if on == gate.OnMerge && rule.Repo == "" {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "merge gate requires --repo")
		}
		if on == gate.OnPromote {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "unknown gate --on promote; use --on merge")
		}
		file, err := gate.Read(home)
		if err != nil {
			return nil, err
		}
		rule.ID = gate.NextID(file.Rules)
		file.Rules = append(file.Rules, rule)
		if err := gate.Write(home, file); err != nil {
			return nil, err
		}
		return rule, nil
	case "gate-ls":
		file, err := gate.Read(home)
		if err != nil {
			return nil, err
		}
		on := FlagString(flags, "on")
		repo := FlagString(flags, "repo")
		catalogID := FlagString(flags, "catalog")
		out := []gate.Rule{}
		for _, rule := range file.Rules {
			if on != "" && rule.On != on {
				continue
			}
			if repo != "" && rule.Repo != repo {
				continue
			}
			if catalogID != "" && rule.Catalog != catalogID {
				continue
			}
			out = append(out, rule)
		}
		return map[string]any{"rules": out}, nil
	case "gate-rm":
		id, err := RequireFlag(flags, "id")
		if err != nil {
			return nil, err
		}
		file, err := gate.Read(home)
		if err != nil {
			return nil, err
		}
		kept := file.Rules[:0]
		found := false
		for _, rule := range file.Rules {
			if rule.ID == id {
				found = true
				continue
			}
			kept = append(kept, rule)
		}
		if !found {
			return nil, fmt.Errorf("unknown gate %s", id)
		}
		file.Rules = kept
		if err := gate.Write(home, file); err != nil {
			return nil, err
		}
		return map[string]any{"revoked": id}, nil
	default:
		return nil, fmt.Errorf("unknown command %s", command)
	}
}
