package invocation

import (
	"crypto/rand"
	"encoding/hex"
	"slices"

	"kc/invoke"
	"kc/kernel"
)

// GrantList is an inventory of explicit rules, not the private authorization
// ledger and not a claim about effective permissions or default access.
type GrantList struct {
	Rules []AllowRule `json:"rules"`
}

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
		target = AllowRule{}
		if stored, count := grantByID(file.Rules, cx.Flag("id")); count == 1 {
			target = stored
		}
	}
	for _, rule := range file.Rules {
		if rule.Principal != cx.Flag("as") || rule.Ref != "" || rule.Object != "" || rule.Aspect != "" {
			continue
		}
		if rule.Repo != "" && rule.Repo != target.Repo || rule.Catalog != "" && rule.Catalog != target.Catalog || rule.Dataset != "" && rule.Dataset != target.Dataset {
			continue
		}
		for _, granted := range rule.Actions {
			if ActionMatches(granted, action) {
				return nil
			}
		}
	}
	return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s in the requested grant scope", cx.Flag("as"), action)
}

func grantByID(rules []AllowRule, id string) (AllowRule, int) {
	var found AllowRule
	count := 0
	for _, rule := range rules {
		if rule.ID == id {
			found = rule
			count++
		}
	}
	return found, count
}

func newGrantRuleID(rules []AllowRule) (string, error) {
	for {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", err
		}
		id := "alw_" + hex.EncodeToString(random[:])
		if _, count := grantByID(rules, id); count == 0 {
			return id, nil
		}
	}
}

func VerbGrantList(cx *Invocation) (any, error) {
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return nil, err
	}
	out := GrantList{Rules: []AllowRule{}}
	filter := grantScope(cx.Flags)
	for _, rule := range file.Rules {
		if !grantMatchesFilter(rule, filter, cx.Flag("principal"), cx.Flag("action")) {
			continue
		}
		rule.Actions = slices.Clone(rule.Actions)
		rule.Cmds = nil
		out.Rules = append(out.Rules, rule)
	}
	return out, nil
}

func grantMatchesFilter(rule, filter AllowRule, principal, action string) bool {
	for _, coordinate := range [][2]string{{principal, rule.Principal}, {filter.Repo, rule.Repo}, {filter.Catalog, rule.Catalog}, {filter.Dataset, rule.Dataset}, {filter.Ref, rule.Ref}, {filter.Object, rule.Object}, {filter.Aspect, rule.Aspect}} {
		if coordinate[0] != "" && coordinate[0] != coordinate[1] {
			return false
		}
	}
	if action == "" {
		return true
	}
	for _, granted := range rule.Actions {
		if ActionMatches(granted, action) {
			return true
		}
	}
	return false
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
