package cli

import (
	"fmt"

	"kc/gate"
	"kc/hook"
	"kc/kernel"
)

// Outbound hooks and merge gates. Two different mechanisms that are easy to
// confuse, so they are registered side by side here:
//
//	hook-*  call a user system before/after a kc verb (docs/HOOKS.md)
//	gate-*  evidence a merge must present about a pinned Preview (docs/GATES.md)
//
// A gate is not a kind of hook: it is a pure check, and it cannot run a suite.

func policyVerbs() map[string]command {
	return map[string]command{
		"hook-add": {stage: stageHome, run: verbHookAdd},
		"hook-ls":  {stage: stageHome, run: verbHookLs},
		"hook-rm":  {stage: stageHome, run: verbHookRm},
		"gate-add": {stage: stageHome, run: verbGateAdd},
		"gate-ls":  {stage: stageHome, run: verbGateLs},
		"gate-rm":  {stage: stageHome, run: verbGateRm},
	}
}

func verbHookAdd(cx *invocation) (any, error) {
	on, err := cx.require("on")
	if err != nil {
		return nil, err
	}
	phase, err := cx.require("phase")
	if err != nil {
		return nil, err
	}
	binding := hook.Binding{
		On:      on,
		Phase:   phase,
		Repo:    cx.flag("repo"),
		Catalog: cx.flag("catalog"),
		Run:     cx.flag("run"),
		URL:     cx.flag("url"),
	}
	if err := hook.ValidateBinding(binding); err != nil {
		return nil, err
	}
	file, err := hook.Read(cx.Home)
	if err != nil {
		return nil, err
	}
	binding.ID = hook.NextID(file.Bindings)
	file.Bindings = append(file.Bindings, binding)
	if err := hook.Write(cx.Home, file); err != nil {
		return nil, err
	}
	return binding, nil
}

func verbHookLs(cx *invocation) (any, error) {
	file, err := hook.Read(cx.Home)
	if err != nil {
		return nil, err
	}
	scope := cx.bindingScope()
	out := []hook.Binding{}
	for _, b := range file.Bindings {
		if scope.matches(b.On, b.Repo, b.Catalog) {
			out = append(out, b)
		}
	}
	return map[string]any{"bindings": out}, nil
}

func verbHookRm(cx *invocation) (any, error) {
	id, err := cx.require("id")
	if err != nil {
		return nil, err
	}
	file, err := hook.Read(cx.Home)
	if err != nil {
		return nil, err
	}
	kept, found := dropByID(file.Bindings, id, func(b hook.Binding) string { return b.ID })
	if !found {
		return nil, fmt.Errorf("unknown hook %s", id)
	}
	file.Bindings = kept
	if err := hook.Write(cx.Home, file); err != nil {
		return nil, err
	}
	return map[string]any{"revoked": id}, nil
}

func verbGateAdd(cx *invocation) (any, error) {
	on, err := cx.require("on")
	if err != nil {
		return nil, err
	}
	if err := gate.ValidateOn(on); err != nil {
		return nil, err
	}
	require := gate.ParseRequire(cx.flag("require"))
	if err := gate.ValidateRequire(require); err != nil {
		return nil, err
	}
	rule := gate.Rule{
		On:      on,
		Repo:    cx.flag("repo"),
		Catalog: cx.flag("catalog"),
		Require: require,
	}
	if on == gate.OnMerge && rule.Repo == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "merge gate requires --repo")
	}
	file, err := gate.Read(cx.Home)
	if err != nil {
		return nil, err
	}
	rule.ID = gate.NextID(file.Rules)
	file.Rules = append(file.Rules, rule)
	if err := gate.Write(cx.Home, file); err != nil {
		return nil, err
	}
	return rule, nil
}

func verbGateLs(cx *invocation) (any, error) {
	file, err := gate.Read(cx.Home)
	if err != nil {
		return nil, err
	}
	scope := cx.bindingScope()
	out := []gate.Rule{}
	for _, rule := range file.Rules {
		if scope.matches(rule.On, rule.Repo, rule.Catalog) {
			out = append(out, rule)
		}
	}
	return map[string]any{"rules": out}, nil
}

func verbGateRm(cx *invocation) (any, error) {
	id, err := cx.require("id")
	if err != nil {
		return nil, err
	}
	file, err := gate.Read(cx.Home)
	if err != nil {
		return nil, err
	}
	kept, found := dropByID(file.Rules, id, func(r gate.Rule) string { return r.ID })
	if !found {
		return nil, fmt.Errorf("unknown gate %s", id)
	}
	file.Rules = kept
	if err := gate.Write(cx.Home, file); err != nil {
		return nil, err
	}
	return map[string]any{"revoked": id}, nil
}

// bindingScope is the shared --on / --repo / --catalog filter for hook-ls and
// gate-ls. An empty field matches everything.
type bindingScope struct {
	on      string
	repo    string
	catalog string
}

func (cx *invocation) bindingScope() bindingScope {
	return bindingScope{on: cx.flag("on"), repo: cx.flag("repo"), catalog: cx.flag("catalog")}
}

func (s bindingScope) matches(on, repo, catalogID string) bool {
	if s.on != "" && s.on != on {
		return false
	}
	if s.repo != "" && s.repo != repo {
		return false
	}
	if s.catalog != "" && s.catalog != catalogID {
		return false
	}
	return true
}
