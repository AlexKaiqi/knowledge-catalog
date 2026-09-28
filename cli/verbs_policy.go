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
//	hook-*  call a user system before/after a kc verb (docs/reviewed/hooks-and-gates.md)
//	gate-*  evidence a merge must present about a pinned Preview (docs/reviewed/hooks-and-gates.md)
//
// A gate is not a kind of hook: it is a pure check, and it cannot run a suite.

func policyVerbs() map[string]command {
	return map[string]command{
		"operations-hook-add":    {Stage: stageHome, Run: verbHookAdd},
		"operations-hook-list":   {Stage: stageHome, Run: verbHookLs},
		"operations-hook-remove": {Stage: stageHome, Run: verbHookRm},
		"operations-gate-add":    {Stage: stageHome, Run: verbGateAdd},
		"operations-gate-list":   {Stage: stageHome, Run: verbGateLs},
		"operations-gate-remove": {Stage: stageHome, Run: verbGateRm},
	}
}

func verbHookAdd(cx *invocation) (any, error) {
	on, err := cx.Require("on")
	if err != nil {
		return nil, err
	}
	phase, err := cx.Require("phase")
	if err != nil {
		return nil, err
	}
	binding := hook.Binding{
		On:      on,
		Phase:   phase,
		Repo:    cx.Flag("repo"),
		Catalog: cx.Flag("catalog"),
		Run:     cx.Flag("run"),
		URL:     cx.Flag("url"),
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
	scope := cx.BindingScope()
	out := []hook.Binding{}
	for _, b := range file.Bindings {
		if scope.Matches(b.On, b.Repo, b.Catalog) {
			out = append(out, b)
		}
	}
	return map[string]any{"bindings": out}, nil
}

func verbHookRm(cx *invocation) (any, error) {
	id, err := cx.Require("id")
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
	on, err := cx.Require("on")
	if err != nil {
		return nil, err
	}
	if err := gate.ValidateOn(on); err != nil {
		return nil, err
	}
	require := gate.ParseRequire(cx.Flag("require"))
	if err := gate.ValidateRequire(require); err != nil {
		return nil, err
	}
	rule := gate.Rule{
		On:      on,
		Repo:    cx.Flag("repo"),
		Catalog: cx.Flag("catalog"),
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
	scope := cx.BindingScope()
	out := []gate.Rule{}
	for _, rule := range file.Rules {
		if scope.Matches(rule.On, rule.Repo, rule.Catalog) {
			out = append(out, rule)
		}
	}
	return map[string]any{"rules": out}, nil
}

func verbGateRm(cx *invocation) (any, error) {
	id, err := cx.Require("id")
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
