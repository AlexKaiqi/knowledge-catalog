package access

import (
	"context"

	"kc/kernel"
)

// RuleStore owns one durable policy authority. UpdateRules applies its
// callback and persists the returned rules under the same serialization
// boundary; a callback error must leave the previous state unchanged. An
// application adapter preserves its private ledger fields in that update.
type RuleStore interface {
	ReadRules(context.Context) ([]Rule, error)
	UpdateRules(context.Context, func([]Rule) ([]Rule, error)) error
}

// Service supplies authenticated explicit-rule administration without Home,
// knowledge interpretation or deployment-default policy. The caller must pass
// the verified principal, never a name asserted in the request body.
type Service struct {
	Store RuleStore
}

func (s Service) ready(principal string) error {
	if principal == "" {
		return kernel.Fail(kernel.ErrUnauthenticated, "an authenticated principal is required")
	}
	if s.Store == nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "durable authorization policy is unavailable")
	}
	return nil
}

func (s Service) List(ctx context.Context, principal string, filter Query) (GrantList, error) {
	if err := s.ready(principal); err != nil {
		return GrantList{}, err
	}
	rules, err := s.Store.ReadRules(ctx)
	if err != nil {
		return GrantList{}, err
	}
	target := Rule{Repo: filter.Repo, Catalog: filter.Catalog, Dataset: filter.Dataset, Ref: filter.Ref, Object: filter.Object, Aspect: filter.Aspect}
	if err := AuthorizeManagement(rules, principal, "admin.grants.read", target); err != nil {
		return GrantList{}, err
	}
	return Inventory(rules, filter), nil
}

func (s Service) Add(ctx context.Context, principal string, requested Rule) (Rule, error) {
	if err := s.ready(principal); err != nil {
		return Rule{}, err
	}
	var receipt Rule
	err := s.Store.UpdateRules(ctx, func(rules []Rule) ([]Rule, error) {
		if err := AuthorizeManagement(rules, principal, "admin.grants.manage", requested); err != nil {
			return nil, err
		}
		updated, created, err := AddRule(rules, requested)
		receipt = created
		return updated, err
	})
	if err != nil {
		return Rule{}, err
	}
	return receipt, nil
}

func (s Service) Revoke(ctx context.Context, principal, id string) error {
	if err := s.ready(principal); err != nil {
		return err
	}
	return s.Store.UpdateRules(ctx, func(rules []Rule) ([]Rule, error) {
		if err := AuthorizeManagement(rules, principal, "admin.grants.manage", RevokeTarget(rules, id)); err != nil {
			return nil, err
		}
		return RemoveRule(rules, id)
	})
}
