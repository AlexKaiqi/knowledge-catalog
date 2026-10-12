package access

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"

	"kc/kernel"
)

// GrantList contains explicit rules, not a private policy ledger or an
// evaluation of deployment defaults and effective permissions.
type GrantList struct {
	Rules []Rule `json:"rules"`
}

// AuthorizeManagement checks explicit management coordinates. Complex
// administrator qualifiers fail closed rather than treating a target glob as
// a concrete value and incorrectly inferring delegated scope containment.
func AuthorizeManagement(rules []Rule, principal, action string, target Rule) error {
	for _, rule := range rules {
		if rule.Principal != principal || rule.Ref != "" || rule.Object != "" || rule.Aspect != "" {
			continue
		}
		if !scopeMatches(rule, Query{Repo: target.Repo, Catalog: target.Catalog, Dataset: target.Dataset}) {
			continue
		}
		if actionAllowed(rule.Actions, action) {
			return nil
		}
	}
	return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s in the requested grant scope", principal, action)
}

func Inventory(rules []Rule, filter Query) GrantList {
	out := GrantList{Rules: []Rule{}}
	for _, rule := range rules {
		if !matchesFilter(rule, filter) {
			continue
		}
		rule.Actions = slices.Clone(rule.Actions)
		rule.Cmds = nil
		out.Rules = append(out.Rules, rule)
	}
	return out
}

func matchesFilter(rule Rule, filter Query) bool {
	for _, coordinate := range [][2]string{{filter.Principal, rule.Principal}, {filter.Repo, rule.Repo}, {filter.Catalog, rule.Catalog}, {filter.Dataset, rule.Dataset}, {filter.Ref, rule.Ref}, {filter.Object, rule.Object}, {filter.Aspect, rule.Aspect}} {
		if coordinate[0] != "" && coordinate[0] != coordinate[1] {
			return false
		}
	}
	return filter.Action == "" || actionAllowed(rule.Actions, filter.Action)
}

func ruleByID(rules []Rule, id string) (Rule, int) {
	var found Rule
	count := 0
	for _, rule := range rules {
		if rule.ID == id {
			found = rule
			count++
		}
	}
	return found, count
}

// RevokeTarget derives scope from one durable rule. Unknown or ambiguous IDs
// remain unscoped so only a global administrator can receive their errors.
func RevokeTarget(rules []Rule, id string) Rule {
	if rule, count := ruleByID(rules, id); count == 1 {
		return rule
	}
	return Rule{}
}

// AddRule validates and constructs an explicit grant. Authorization belongs to
// Service or the calling application's trusted operation boundary.
func AddRule(rules []Rule, requested Rule) ([]Rule, Rule, error) {
	if requested.Principal == "" {
		return nil, Rule{}, fmt.Errorf("grant add requires principal")
	}
	if err := ValidateActions(requested.Actions); err != nil {
		return nil, Rule{}, err
	}
	if (requested.Repo == "") == (requested.Catalog == "") {
		return nil, Rule{}, fmt.Errorf("grant add requires exactly one of --repo or --catalog")
	}
	id, err := newRuleID(rules)
	if err != nil {
		return nil, Rule{}, err
	}
	rule := Rule{
		ID: id, Principal: requested.Principal, Actions: slices.Clone(requested.Actions),
		Repo: requested.Repo, Catalog: requested.Catalog, Ref: requested.Ref,
		Object: requested.Object, Aspect: requested.Aspect, Dataset: requested.Dataset,
	}
	updated := append(slices.Clone(rules), rule)
	rule.Actions = slices.Clone(rule.Actions)
	return updated, rule, nil
}

func newRuleID(rules []Rule) (string, error) {
	for {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", err
		}
		id := "alw_" + hex.EncodeToString(random[:])
		if _, count := ruleByID(rules, id); count == 0 {
			return id, nil
		}
	}
}

func RemoveRule(rules []Rule, id string) ([]Rule, error) {
	if id == "" {
		return nil, fmt.Errorf("missing --id")
	}
	_, count := ruleByID(rules, id)
	if count == 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown rule %s", id)
	}
	if count != 1 {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "grant ID %s is ambiguous; repair the authorization ledger explicitly", id)
	}
	kept := make([]Rule, 0, len(rules)-1)
	for _, rule := range rules {
		if rule.ID != id {
			kept = append(kept, rule)
		}
	}
	return kept, nil
}
