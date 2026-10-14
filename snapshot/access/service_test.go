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

func cloneRules(rules []access.Rule) []access.Rule {
	cloned := slices.Clone(rules)
	for i := range cloned {
		cloned[i].Actions = slices.Clone(rules[i].Actions)
		cloned[i].Cmds = slices.Clone(rules[i].Cmds)
	}
	return cloned
}

func (m *memoryRules) ReadRules(context.Context) ([]access.Rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneRules(m.rules), m.failure
}

func (m *memoryRules) UpdateRules(_ context.Context, update func([]access.Rule) ([]access.Rule, error)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rules, err := update(cloneRules(m.rules))
	if err != nil {
		return err
	}
	if m.failure != nil {
		return m.failure
	}
	m.rules = cloneRules(rules)
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
	before := cloneRules(store.rules)
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
	before := cloneRules(store.rules)
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
	before := cloneRules(store.rules)
	store.failure = errors.New("storage offline")
	if receipt, err := service.Add(ctx, "manager", requested); !errors.Is(err, store.failure) || !reflect.DeepEqual(receipt, access.Rule{}) {
		t.Fatalf("failed persistence returned a grant receipt: %#v %v", receipt, err)
	}
	if err := service.Revoke(ctx, "global", "repository-manager"); !errors.Is(err, store.failure) {
		t.Fatalf("failed persistence changed or acknowledged revoke: %v", err)
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

func TestDatasetGrantAdministrationUsesTheExactDurableScope(t *testing.T) {
	for _, test := range []struct {
		operation string
		dataset   string
	}{
		{"add", "one"}, {"add", "two"}, {"revoke", "one"}, {"revoke", "two"},
	} {
		t.Run(test.operation+"-"+test.dataset, func(t *testing.T) {
			store := &memoryRules{rules: []access.Rule{
				{ID: "manager", Principal: "editor", Actions: []string{"admin.grants.manage"}, Dataset: "one"},
				{ID: "target-one", Principal: "reader", Actions: []string{"file.read"}, Dataset: "one"},
				{ID: "target-two", Principal: "other", Actions: []string{"file.read"}, Dataset: "two"},
			}}
			service := access.NewService(store)
			before, want := cloneRules(store.rules), cloneRules(store.rules)
			var err error
			if test.operation == "add" {
				requested := access.Rule{Principal: "recipient", Actions: []string{"dataset.resolve"}, Dataset: test.dataset}
				var receipt access.Rule
				receipt, err = service.Add(context.Background(), "editor", requested)
				if test.dataset == "one" {
					if receipt.ID == "" {
						t.Fatal("successful grant has no durable identity")
					}
					requested.ID = receipt.ID
					if !reflect.DeepEqual(receipt, requested) {
						t.Fatalf("grant widened or changed requested scope: %#v", receipt)
					}
					want = append(want, requested)
				} else if !reflect.DeepEqual(receipt, access.Rule{}) {
					t.Fatalf("denied grant has a receipt: %#v", receipt)
				}
			} else {
				err = service.Revoke(context.Background(), "editor", "target-"+test.dataset)
				if test.dataset == "one" {
					want = append(want[:1], want[2:]...)
				}
			}
			if test.dataset == "one" && err != nil || test.dataset == "two" && kernel.CodeOf(err) != kernel.ErrForbidden {
				t.Fatalf("Dataset management crossed scope: %v", err)
			}
			if !reflect.DeepEqual(store.rules, want) {
				t.Fatalf("scope management changed unrelated rules: before=%#v want=%#v got=%#v", before, want, store.rules)
			}
		})
	}
}

func TestConstrainedManagementRulesDoNotDelegateAuthority(t *testing.T) {
	for _, test := range []struct {
		name  string
		alter func(*access.Rule)
	}{
		{"ref", func(rule *access.Rule) { rule.Ref = "main" }},
		{"object", func(rule *access.Rule) { rule.Object = "data/*" }},
		{"aspect", func(rule *access.Rule) { rule.Aspect = "canonical" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, store := scopedService()
			test.alter(&store.rules[1])
			before := cloneRules(store.rules)
			requested := access.Rule{Principal: "consumer", Actions: []string{"file.read"}, Repo: "A", Ref: "main", Object: "data/table", Aspect: "canonical"}
			if receipt, err := service.Add(context.Background(), "manager", requested); kernel.CodeOf(err) != kernel.ErrForbidden || !reflect.DeepEqual(receipt, access.Rule{}) {
				t.Fatalf("constrained manager delegated authority: %#v %v", receipt, err)
			}
			if !reflect.DeepEqual(store.rules, before) {
				t.Fatal("denied delegation changed durable policy")
			}
		})
	}
}
