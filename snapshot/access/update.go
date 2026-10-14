package access

import (
	"context"
	"reflect"
	"slices"

	"kc/kernel"
)

func (s *service) Update(ctx context.Context, principal, id string, expected, requested Rule) (Rule, error) {
	if err := s.ready(principal); err != nil {
		return Rule{}, err
	}
	var receipt Rule
	err := s.store.UpdateRules(ctx, func(rules []Rule) ([]Rule, error) {
		target := RevokeTarget(rules, id)
		if id == "" {
			target = Rule{}
		}
		if err := AuthorizeManagement(rules, principal, "admin.grants.manage", target); err != nil {
			return nil, err
		}
		if id == "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "grant update requires ID")
		}
		old, count := ruleByID(rules, id)
		if count == 0 {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown rule %s", id)
		}
		if count != 1 {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "grant ID %s is ambiguous", id)
		}
		if err := AuthorizeManagement(rules, principal, "admin.grants.manage", requested); err != nil {
			return nil, err
		}
		publicOld := old
		if len(publicOld.Actions) == 0 {
			publicOld.Actions = nil
		}
		if len(expected.Actions) == 0 {
			expected.Actions = nil
		}
		if !reflect.DeepEqual(publicOld, expected) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "grant changed; reload before editing")
		}
		if old.ShareID != "" {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "shared grants must be updated through share management")
		}
		if err := validateReplacement(old, requested); err != nil {
			return nil, err
		}
		receipt = requested
		receipt.ID, receipt.SharedBy, receipt.ShareID = old.ID, old.SharedBy, old.ShareID
		receipt.Actions = slices.Clone(requested.Actions)
		updated := slices.Clone(rules)
		for i := range updated {
			if updated[i].ID == id {
				updated[i] = receipt
				break
			}
		}
		return updated, nil
	})
	if err != nil {
		return Rule{}, err
	}
	receipt.Actions = slices.Clone(receipt.Actions)
	return receipt, nil
}

func validateReplacement(old, requested Rule) error {
	if requested.Principal == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "grant update requires principal")
	}
	actions := requested.Actions
	if slices.Contains(old.Actions, "*") {
		actions = slices.DeleteFunc(slices.Clone(actions), func(action string) bool { return action == "*" })
	}
	if len(requested.Actions) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "grant update requires actions")
	}
	if len(actions) > 0 {
		if err := ValidateActions(actions); err != nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "%s", err)
		}
	}
	return nil
}
