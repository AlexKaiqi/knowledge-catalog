package invocation

import (
	"fmt"

	"kc/invoke"
	"kc/kernel"
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
	actions := SplitCmds(cx.Flag("action"))
	if err := ValidateActions(actions); err != nil {
		return nil, err
	}
	repo := cx.Flag("repo")
	catalogID := cx.Flag("catalog")
	if (repo == "") == (catalogID == "") {
		return nil, fmt.Errorf("grant add requires exactly one of --repo or --catalog")
	}
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return nil, err
	}
	id, err := newGrantRuleID(file.Rules)
	if err != nil {
		return nil, err
	}
	rule := AllowRule{
		ID:        id,
		Principal: principal,
		Actions:   actions,
		Repo:      repo,
		Catalog:   catalogID,
		Ref:       cx.Flag("ref"),
		Object:    cx.Flag("object"),
		Aspect:    cx.Flag("aspect"),
		Dataset:   invoke.DatasetOf(cx.Flags),
	}
	file.Rules = append(file.Rules, rule)
	if err := WriteAllow(cx.Home, file); err != nil {
		return nil, err
	}
	return rule, nil
}

func VerbRevoke(cx *Invocation) (any, error) {
	id, err := cx.Require("id")
	if err != nil {
		return nil, err
	}
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return nil, err
	}
	_, count := grantByID(file.Rules, id)
	if count == 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown rule %s", id)
	}
	if count != 1 {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "grant ID %s is ambiguous; repair the authorization ledger explicitly", id)
	}
	kept, _ := DropByID(file.Rules, id, func(r AllowRule) string { return r.ID })
	file.Rules = kept
	if err := WriteAllow(cx.Home, file); err != nil {
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
		Catalog:   cx.Flag("catalog"),
		Ref:       cx.Flag("ref"),
		Object:    cx.Flag("object"),
		Aspect:    cx.Flag("aspect"),
		Dataset:   invoke.DatasetOf(cx.Flags),
	})
}
