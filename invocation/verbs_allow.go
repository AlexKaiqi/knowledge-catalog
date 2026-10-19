package invocation

import (
	"kc/invoke"
	"kc/snapshot/access"
)

func VerbWhoami(cx *Invocation) (any, error) {
	identity, err := IdentityContextFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"principal": identity.Principal}
	if identity.OnBehalfOf != "" {
		out["onBehalfOf"] = identity.OnBehalfOf
	}
	if cx.Flag("auth-login") != "" {
		out["login"] = cx.Flag("auth-login")
	}
	return out, nil
}

func VerbAllow(cx *Invocation) (any, error) {
	principal, err := cx.Require("principal")
	if err != nil {
		return nil, err
	}
	var rule AllowRule
	err = UpdateAllow(cx.Home, func(file *AllowFile) error {
		updated, added, err := access.AddRule(file.Rules, AllowRule{
			Principal: principal, Actions: SplitCmds(cx.Flag("action")),
			Repo: cx.Flag("repo"),
			Ref:  cx.Flag("ref"), Object: cx.Flag("object"), Aspect: cx.Flag("aspect"),
			Dataset: invoke.DatasetOf(cx.Flags),
		})
		if err != nil {
			return err
		}
		file.Rules, rule = updated, added
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rule, nil
}

func VerbRevoke(cx *Invocation) (any, error) {
	id, err := cx.Require("id")
	if err != nil {
		return nil, err
	}
	err = UpdateAllow(cx.Home, func(file *AllowFile) error {
		updated, err := access.RemoveRule(file.Rules, id)
		if err != nil {
			return err
		}
		file.Rules = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"revoked": id}, nil
}

// VerbAllowed preserves the CLI's explicit-rule evaluation when action is
// supplied. HTTP inventory calls VerbGrantList independently of this question.
func VerbAllowed(cx *Invocation) (any, error) {
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return nil, err
	}
	principal := cx.Flag("principal")
	if principal == "" {
		principal = cx.Flag("as")
	}
	action := cx.Flag("action")
	if principal == "" || action == "" {
		return VerbGrantList(cx)
	}
	return EvaluateAllow(file.Rules, AllowQuery{
		Principal: principal,
		Action:    action,
		Repo:      cx.Flag("repo"),
		Ref:       cx.Flag("ref"),
		Object:    cx.Flag("object"),
		Aspect:    cx.Flag("aspect"),
		Dataset:   invoke.DatasetOf(cx.Flags),
	})
}
