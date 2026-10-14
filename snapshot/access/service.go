package access

import (
	"context"

	"kc/kernel"
)

// Service supplies authenticated explicit-rule administration without Home,
// object interpretation or deployment-default policy. The caller must pass
// the verified principal, never a name asserted in the request body.
type service struct{ store RuleStore }

func NewService(store RuleStore) Service { return &service{store: store} }

func (s *service) Rules(ctx context.Context) ([]Rule, error) {
	if s == nil || s.store == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "durable authorization policy is unavailable")
	}
	return s.store.ReadRules(ctx)
}

func (s *service) ready(principal string) error {
	if principal == "" {
		return kernel.Fail(kernel.ErrUnauthenticated, "an authenticated principal is required")
	}
	if s.store == nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "durable authorization policy is unavailable")
	}
	return nil
}

func (s *service) List(ctx context.Context, principal string, filter Query) (GrantList, error) {
	if err := s.ready(principal); err != nil {
		return GrantList{}, err
	}
	rules, err := s.store.ReadRules(ctx)
	if err != nil {
		return GrantList{}, err
	}
	target := Rule{Repo: filter.Repo, Dataset: filter.Dataset, Ref: filter.Ref, Object: filter.Object, Aspect: filter.Aspect}
	if err := AuthorizeManagement(rules, principal, "admin.grants.read", target); err != nil {
		return GrantList{}, err
	}
	return Inventory(rules, filter), nil
}

func (s *service) Add(ctx context.Context, principal string, requested Rule) (Rule, error) {
	if err := s.ready(principal); err != nil {
		return Rule{}, err
	}
	var receipt Rule
	err := s.store.UpdateRules(ctx, func(rules []Rule) ([]Rule, error) {
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

func (s *service) Revoke(ctx context.Context, principal, id string) error {
	if err := s.ready(principal); err != nil {
		return err
	}
	return s.store.UpdateRules(ctx, func(rules []Rule) ([]Rule, error) {
		if err := AuthorizeManagement(rules, principal, "admin.grants.manage", RevokeTarget(rules, id)); err != nil {
			return nil, err
		}
		return RemoveRule(rules, id)
	})
}
