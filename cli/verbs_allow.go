package cli

import (
	"fmt"

	"kc/kernel"
)

// Grant verbs. A grant is over kc verbs on a knowledge repository or a Catalog,
// evaluated against .kc/allow.json. Table-level GRANT is knowledge, enforced by
// the source system, and never lands here. See docs/PERMISSIONS.md.
//
// These stay at stageHome: answering "who may do what" must not require the
// workspace to mount.

func allowVerbs() map[string]command {
	return map[string]command{
		"whoami":  {stage: stageHome, run: verbWhoami},
		"allow":   {stage: stageHome, run: verbAllow},
		"revoke":  {stage: stageHome, run: verbRevoke},
		"allowed": {stage: stageHome, run: verbAllowed},
	}
}

func verbWhoami(cx *invocation) (any, error) {
	identity, err := identityContextFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"principal": identity.Principal}
	if identity.OnBehalfOf != "" {
		out["onBehalfOf"] = identity.OnBehalfOf
	}
	if provider := cx.flag("auth-provider"); provider != "" {
		out["provider"] = provider
		out["subject"] = cx.flag("auth-subject")
		out["login"] = cx.flag("auth-login")
	}
	return out, nil
}

func verbAllow(cx *invocation) (any, error) {
	principal, err := cx.require("principal")
	if err != nil {
		return nil, err
	}
	cmds := splitCmds(cx.flag("cmd"))
	if err := validateCmds(cmds); err != nil {
		return nil, err
	}
	repo := cx.flag("repo")
	catalogID := cx.flag("catalog")
	if repo == "" && catalogID == "" {
		return nil, fmt.Errorf("allow requires --repo or --catalog")
	}
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return nil, err
	}
	rule := AllowRule{
		ID:        nextRuleID(file.Rules),
		Principal: principal,
		Cmds:      cmds,
		Repo:      repo,
		Catalog:   catalogID,
		Ref:       cx.flag("ref"),
		Object:    cx.flag("object"),
		Aspect:    cx.flag("aspect"),
		Workspace: workspaceIDOf(cx.Flags),
	}
	file.Rules = append(file.Rules, rule)
	if err := WriteAllow(cx.Home, file); err != nil {
		return nil, err
	}
	return rule, nil
}

func verbRevoke(cx *invocation) (any, error) {
	id, err := cx.require("id")
	if err != nil {
		return nil, err
	}
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return nil, err
	}
	kept, found := dropByID(file.Rules, id, func(r AllowRule) string { return r.ID })
	if !found {
		return nil, fmt.Errorf("unknown rule %s", id)
	}
	file.Rules = kept
	if err := WriteAllow(cx.Home, file); err != nil {
		return nil, err
	}
	return map[string]any{"revoked": id}, nil
}

// verbAllowed with no principal/cmd dumps the rule file; with both it evaluates
// one question and fails closed.
func verbAllowed(cx *invocation) (any, error) {
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return nil, err
	}
	principal := cx.flag("principal")
	if principal == "" {
		principal = cx.flag("as")
	}
	cmd := cx.flag("cmd")
	if principal == "" || cmd == "" {
		return file, nil
	}
	rule, ok := MatchAllow(file.Rules, AllowQuery{
		Principal: principal,
		Cmd:       cmd,
		Repo:      cx.flag("repo"),
		Catalog:   cx.flag("catalog"),
		Ref:       cx.flag("ref"),
		Object:    cx.flag("object"),
		Aspect:    cx.flag("aspect"),
		Workspace: workspaceIDOf(cx.Flags),
	})
	if !ok {
		return nil, kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", principal, cmd)
	}
	return map[string]any{"allow": true, "ruleId": rule.ID}, nil
}
