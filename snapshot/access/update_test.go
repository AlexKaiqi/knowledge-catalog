package access_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"kc/kernel"
	"kc/snapshot/access"
)

func TestGrantUpdatePreservesPolicyOnFailureAndRejectsSharedOrAmbiguousTargets(t *testing.T) {
	ctx := context.Background()
	offline := errors.New("offline")
	for _, test := range []struct {
		name string
		code kernel.ErrorCode
	}{
		{"read-only", kernel.ErrForbidden},
		{"other-scope", kernel.ErrForbidden},
		{"stale", kernel.ErrPreconditionFailed},
		{"shared", kernel.ErrPreconditionFailed},
		{"ambiguous", kernel.ErrPreconditionFailed},
		{"ambiguous-scoped", kernel.ErrForbidden},
		{name: "durability"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, store := scopedService()
			old := access.Rule{ID: "editable", Principal: "consumer", Actions: []string{"file.read"}, Repo: "A"}
			store.rules = append(store.rules, old)
			requested := old
			requested.Actions = []string{"dataset.resolve"}
			actor := "manager"
			switch test.name {
			case "read-only":
				actor = "reader"
			case "other-scope":
				requested.Repo = "B"
			case "stale":
				old.Actions = []string{"knowledge.read"}
			case "shared":
				store.rules[len(store.rules)-1].ShareID = "share"
				old.ShareID = "share"
			case "ambiguous":
				actor = "global"
				store.rules = append(store.rules, old)
			case "ambiguous-scoped":
				store.rules = append(store.rules, old)
			case "durability":
				store.failure = offline
			}
			before := cloneRules(store.rules)
			receipt, err := service.Update(ctx, actor, "editable", old, requested)
			if test.name == "durability" {
				if !errors.Is(err, offline) {
					t.Fatalf("persistence failure changed: %v", err)
				}
			} else if kernel.CodeOf(err) != test.code {
				t.Fatalf("want %s, got %v", test.code, err)
			}
			if !reflect.DeepEqual(receipt, access.Rule{}) {
				t.Fatalf("failed update returned receipt: %#v", receipt)
			}
			if !reflect.DeepEqual(before, store.rules) {
				t.Fatal("failed edit changed durable policy")
			}
		})
	}
}

func TestGrantUpdateRequiresCurrentAuthorityOverBothDatasetScopes(t *testing.T) {
	for _, test := range []struct {
		name       string
		oldAllowed bool
		newAllowed bool
	}{
		{"both", true, true},
		{"old-only", true, false},
		{"new-only", false, true},
		{"neither", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			old := access.Rule{ID: "target", Principal: "reader", Actions: []string{"file.read"}, Repo: "A", Dataset: "one", Ref: "main", Object: "opaque/*", Aspect: "canonical", SharedBy: "publisher"}
			store := &memoryRules{rules: []access.Rule{old, {ID: "unrelated", Principal: "other", Actions: []string{"file.read"}, Dataset: "other"}}}
			for _, scope := range []struct {
				dataset string
				allowed bool
			}{{"one", test.oldAllowed}, {"two", test.newAllowed}} {
				if scope.allowed {
					store.rules = append(store.rules, access.Rule{ID: "manage-" + scope.dataset, Principal: "editor", Actions: []string{"admin.grants.manage"}, Dataset: scope.dataset})
				}
			}
			before := cloneRules(store.rules)
			requested := access.Rule{Principal: "recipient", Actions: []string{"dataset.resolve"}, Repo: "A", Dataset: "two"}
			receipt, err := access.NewService(store).Update(context.Background(), "editor", old.ID, old, requested)
			want := cloneRules(before)
			if test.oldAllowed && test.newAllowed {
				requested.ID, requested.SharedBy = old.ID, old.SharedBy
				if err != nil || !reflect.DeepEqual(receipt, requested) {
					t.Fatalf("full replacement lost identity, provenance or omitted qualifiers: %#v %v", receipt, err)
				}
				want[0] = requested
			} else if kernel.CodeOf(err) != kernel.ErrForbidden || !reflect.DeepEqual(receipt, access.Rule{}) {
				t.Fatalf("one-sided authority edited the grant: %#v %v", receipt, err)
			}
			if !reflect.DeepEqual(store.rules, want) {
				t.Fatalf("update changed the wrong durable state: want=%#v got=%#v", want, store.rules)
			}
		})
	}
}

func TestConcurrentGrantEditorsCannotOverwriteEachOther(t *testing.T) {
	service, store := scopedService()
	old := access.Rule{ID: "editable", Principal: "consumer", Actions: []string{"file.read"}, Repo: "A", SharedBy: "publisher"}
	store.rules = append(store.rules, old)
	before := cloneRules(store.rules)
	type result struct {
		requested access.Rule
		receipt   access.Rule
		err       error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for _, action := range []string{"dataset.resolve", "dataset.manage"} {
		workers.Add(1)
		go func(action string) {
			defer workers.Done()
			next := old
			next.Actions = []string{action}
			<-start
			receipt, err := service.Update(context.Background(), "manager", old.ID, old, next)
			results <- result{requested: next, receipt: receipt, err: err}
		}(action)
	}
	close(start)
	workers.Wait()
	close(results)
	passed, conflicts := 0, 0
	want := cloneRules(before)
	for result := range results {
		if result.err == nil {
			passed++
			if !reflect.DeepEqual(result.receipt, result.requested) {
				t.Fatalf("winner received a different replacement: %#v %#v", result.receipt, result.requested)
			}
			want[len(want)-1] = result.requested
		} else if kernel.CodeOf(result.err) == kernel.ErrPreconditionFailed && reflect.DeepEqual(result.receipt, access.Rule{}) {
			conflicts++
		} else {
			t.Fatalf("unexpected editor result: %#v %v", result.receipt, result.err)
		}
	}
	if passed != 1 || conflicts != 1 || !reflect.DeepEqual(store.rules, want) {
		t.Fatalf("editors overwrote or duplicated rules: %d %d %#v", passed, conflicts, store.rules)
	}
}

func TestEditingOwnManagementRuleDoesNotRetainOldAuthority(t *testing.T) {
	service, store := scopedService()
	old := store.rules[1]
	next := old
	next.Actions = []string{"file.read"}
	updated, err := service.Update(context.Background(), "manager", old.ID, old, next)
	if err != nil || !reflect.DeepEqual(updated, next) || !reflect.DeepEqual(store.rules[1], next) {
		t.Fatalf("self-demotion did not replace the durable rule: %#v %v", updated, err)
	}
	before := cloneRules(store.rules)
	if receipt, err := service.Update(context.Background(), "manager", old.ID, updated, old); kernel.CodeOf(err) != kernel.ErrForbidden || !reflect.DeepEqual(receipt, access.Rule{}) {
		t.Fatalf("old editor restored revoked management: %#v %v", receipt, err)
	}
	if !reflect.DeepEqual(store.rules, before) {
		t.Fatal("rejected self-elevation changed durable authority")
	}
}
