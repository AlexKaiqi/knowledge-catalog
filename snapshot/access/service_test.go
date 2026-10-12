package access_test

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"slices"
	"sync"
	"testing"

	"kc/kernel"
	"kc/snapshot/access"
)

type memoryRules struct {
	mu      sync.Mutex
	rules   []access.Rule
	failure error
}

func (m *memoryRules) ReadRules(context.Context) ([]access.Rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.rules), m.failure
}

func (m *memoryRules) UpdateRules(_ context.Context, update func([]access.Rule) ([]access.Rule, error)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rules, err := update(slices.Clone(m.rules))
	if err != nil {
		return err
	}
	if m.failure != nil {
		return m.failure
	}
	m.rules = rules
	return nil
}

func scopedService() (access.Service, *memoryRules) {
	store := &memoryRules{rules: []access.Rule{
		{ID: "bootstrap", Principal: "global", Actions: []string{"admin.grants.*"}},
		{ID: "repository-manager", Principal: "manager", Actions: []string{"admin.grants.manage"}, Repo: "A"},
		{ID: "repository-reader", Principal: "reader", Actions: []string{"admin.grants.read"}, Repo: "A"},
		{ID: "constraint-manager", Principal: "limited", Actions: []string{"admin.grants.manage"}, Repo: "A", Object: "data/*"},
		{ID: "B-rule", Principal: "consumer", Actions: []string{"file.read"}, Repo: "B", Dataset: "published"},
	}}
	return access.NewService(store), store
}

func TestServiceSeparatesInventoryManageAndCrossResourceScopes(t *testing.T) {
	service, store := scopedService()
	ctx := context.Background()
	view, err := service.List(ctx, "reader", access.Query{Repo: "A"})
	if err != nil || len(view.Rules) != 3 {
		t.Fatalf("scoped inventory: %#v %v", view, err)
	}
	for _, filter := range []access.Query{{}, {Repo: "B"}, {Repo: "source"}} {
		if _, err := service.List(ctx, "reader", filter); kernel.CodeOf(err) != kernel.ErrForbidden {
			t.Fatalf("inventory leaked another resource: %#v %v", filter, err)
		}
	}
	requested := access.Rule{Principal: "consumer", Actions: []string{"file.read", "dataset.resolve"}, Repo: "A", Dataset: "published"}
	before := slices.Clone(store.rules)
	for _, principal := range []string{"reader", "limited", "native-admin"} {
		if _, err := service.Add(ctx, principal, requested); kernel.CodeOf(err) != kernel.ErrForbidden {
			t.Fatalf("unauthorized add by %s: %v", principal, err)
		}
	}
	requested.Repo = "B"
	if _, err := service.Add(ctx, "manager", requested); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("cross-Repository add: %v", err)
	}
	requested.Repo = "source"
	if _, err := service.Add(ctx, "manager", requested); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("Repo manager granted Repository access: %v", err)
	}
	if err := service.Revoke(ctx, "manager", "B-rule"); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("cross-Repository revoke: %v", err)
	}
	if !reflect.DeepEqual(before, store.rules) {
		t.Fatal("denied administration changed the authority")
	}
	requested.Repo = "A"
	receipt, err := service.Add(ctx, "manager", requested)
	if err != nil || !regexp.MustCompile(`^alw_[a-f0-9]{32}$`).MatchString(receipt.ID) {
		t.Fatalf("manage-only caller could not create a scoped grant: %#v %v", receipt, err)
	}
	if _, err := service.List(ctx, "manager", access.Query{Repo: "A"}); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("manage implied inventory read: %v", err)
	}
	if err := service.Revoke(ctx, "manager", receipt.ID); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, store.rules) {
		t.Fatal("grant receipt did not identify the exact rule to revoke")
	}
}

func TestServiceDerivesRevokeAuthorityFromUniqueDurableTarget(t *testing.T) {
	service, store := scopedService()
	ctx := context.Background()
	store.rules = append(store.rules,
		access.Rule{ID: "legacy", Principal: "one", Repo: "A"},
		access.Rule{ID: "legacy", Principal: "two", Repo: "B"})
	before := slices.Clone(store.rules)
	for _, id := range []string{"missing", "legacy"} {
		if err := service.Revoke(ctx, "manager", id); kernel.CodeOf(err) != kernel.ErrForbidden {
			t.Fatalf("scoped manager learned an unscoped target: %s %v", id, err)
		}
	}
	if err := service.Revoke(ctx, "global", "legacy"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("ambiguous legacy ID was revoked: %v", err)
	}
	if err := service.Revoke(ctx, "global", "missing"); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("unknown global target: %v", err)
	}
	if !reflect.DeepEqual(before, store.rules) {
		t.Fatal("unknown or ambiguous revoke mutated rules")
	}
	if err := service.Revoke(ctx, "manager", "repository-manager"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Add(ctx, "manager", access.Rule{Principal: "manager", Actions: []string{"admin.grants.manage"}, Repo: "A"}); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("a revoked administrator continued using cached authority: %v", err)
	}
}

func TestServiceAcknowledgesOnlyDurableSuccessAndRequiresIdentity(t *testing.T) {
	service, store := scopedService()
	ctx := context.Background()
	requested := access.Rule{Principal: "consumer", Actions: []string{"file.read"}, Repo: "A"}
	before := slices.Clone(store.rules)
	store.failure = errors.New("storage offline")
	if receipt, err := service.Add(ctx, "manager", requested); err == nil || receipt.ID != "" {
		t.Fatalf("failed persistence returned a grant receipt: %#v %v", receipt, err)
	}
	if err := service.Revoke(ctx, "global", "repository-manager"); err == nil {
		t.Fatal("failed persistence acknowledged revoke")
	}
	if !reflect.DeepEqual(before, store.rules) {
		t.Fatal("failed persistence changed rules")
	}
	store.failure = nil
	if _, err := service.Add(ctx, "", requested); kernel.CodeOf(err) != kernel.ErrUnauthenticated {
		t.Fatalf("independent service inherited local-owner bypass: %v", err)
	}
	if _, err := access.NewService(nil).List(ctx, "reader", access.Query{}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("missing policy became empty authority: %v", err)
	}
}
