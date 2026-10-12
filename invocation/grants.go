package invocation

import (
	"kc/invoke"
	"kc/kernel"
	"kc/snapshot/access"
)

// GrantList is an inventory of explicit rules, not the private authorization
// ledger and not a claim about effective permissions or default access.
type GrantList = access.GrantList

func grantScope(flags map[string]invoke.FlagValue) AllowRule {
	return AllowRule{Repo: invoke.FlagString(flags, "repo"), Catalog: invoke.FlagString(flags, "catalog"), Dataset: invoke.DatasetOf(flags), Ref: invoke.FlagString(flags, "ref"), Object: invoke.FlagString(flags, "object"), Aspect: invoke.FlagString(flags, "aspect")}
}

// AuthorizeGrantAdministration uses explicit resource coordinates. Grant
// removal derives its coordinates from the durable rule, never a caller's
// repeated scope or the Home's first Catalog. Complex administrator constraints
// fail closed: matching a target glob as a concrete object cannot establish
// that the delegated scope is contained in the administrator's scope.
func AuthorizeGrantAdministration(cx *Invocation, action string) (authErr error) {
	defer observeAuthorizationResult(invoke.NoObservationTelemetry(cx.Observation).Authorization, &authErr)()
	if OwnerBypass(cx.Flags) {
		return nil
	}
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return err
	}
	target := grantScope(cx.Flags)
	if action == "admin.grants.read" && cx.Command == "grant-list" && cx.Flag("action") != "" {
		// The legacy CLI question evaluates all applicable explicit rules,
		// including global grants. A scoped inventory cannot safely answer it.
		target = AllowRule{}
	}
	if action == "admin.grants.manage" && (cx.Command == "grant-remove" || cx.Command == "admin-grant-remove") {
		target = access.RevokeTarget(file.Rules, cx.Flag("id"))
	}
	return access.AuthorizeManagement(file.Rules, cx.Flag("as"), action, target)
}

func VerbGrantList(cx *Invocation) (any, error) {
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return nil, err
	}
	return access.Inventory(file.Rules, AllowQuery{
		Principal: cx.Flag("principal"), Action: cx.Flag("action"),
		Repo: cx.Flag("repo"), Catalog: cx.Flag("catalog"), Dataset: invoke.DatasetOf(cx.Flags),
		Ref: cx.Flag("ref"), Object: cx.Flag("object"), Aspect: cx.Flag("aspect"),
	}), nil
}

// EvaluateAllow retains the existing CLI's explicit-rule question. It does
// not include deployment defaults and does not create an authorization route.
func EvaluateAllow(rules []AllowRule, query AllowQuery) (any, error) {
	rule, ok := MatchAllow(rules, query)
	if !ok {
		return nil, kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", query.Principal, query.Action)
	}
	return map[string]any{"allow": true, "ruleId": rule.ID}, nil
}
