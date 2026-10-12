// Package access owns explicit scoped policy and its durable management port.
package access

import "context"

// RuleStore owns one durable policy authority. UpdateRules applies its
// callback and persists the returned rules under the same serialization
// boundary; a callback error must leave the previous state unchanged. An
// application adapter preserves its private ledger fields in that update.
type RuleStore interface {
	ReadRules(context.Context) ([]Rule, error)
	UpdateRules(context.Context, func([]Rule) ([]Rule, error)) error
}

// Service manages the caller's explicit grants against one RuleStore.
type Service interface {
	Rules(context.Context) ([]Rule, error)
	List(context.Context, string, Query) (GrantList, error)
	Add(context.Context, string, Rule) (Rule, error)
	// Update replaces one unique rule only if expected still matches its public
	// snapshot. Both the old and proposed scopes require current management.
	Update(context.Context, string, string, Rule, Rule) (Rule, error)
	Revoke(context.Context, string, string) error
}
