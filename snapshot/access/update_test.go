package access_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"

	"kc/kernel"
	"kc/snapshot/access"
)

func TestGrantUpdatePreservesPolicyOnFailureAndRejectsSharedOrAmbiguousTargets(t *testing.T) {
	ctx := context.Background()
	for _, scenario := range []string{"read-only", "other-scope", "stale", "shared", "ambiguous", "durability"} {
		t.Run(scenario, func(t *testing.T) {
			service, store := scopedService()
			old := access.Rule{ID: "editable", Principal: "consumer", Actions: []string{"file.read"}, Repo: "A"}
			store.rules = append(store.rules, old)
			requested := old
			requested.Actions = []string{"dataset.resolve"}
			actor := "manager"
			switch scenario {
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
				store.rules = append(store.rules, old)
			case "durability":
				store.failure = errors.New("offline")
			}
			before := slices.Clone(store.rules)
			if receipt, err := service.Update(ctx, actor, "editable", old, requested); err == nil || receipt.ID != "" {
				t.Fatalf("failed update returned receipt: %#v %v", receipt, err)
			}
			if !reflect.DeepEqual(before, store.rules) {
				t.Fatal("failed edit changed durable policy")
			}
		})
	}
}

func TestConcurrentGrantEditorsCannotOverwriteEachOther(t *testing.T) {
	service, store := scopedService()
	old := access.Rule{ID: "editable", Principal: "consumer", Actions: []string{"file.read"}, Repo: "A"}
	store.rules = append(store.rules, old)
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	for _, action := range []string{"dataset.resolve", "dataset.manage"} {
		workers.Add(1)
		go func(action string) {
			defer workers.Done()
			next := old
			next.Actions = []string{action}
			_, err := service.Update(context.Background(), "manager", old.ID, old, next)
			errors <- err
		}(action)
	}
	workers.Wait()
	close(errors)
	passed, conflicts := 0, 0
	for err := range errors {
		if err == nil {
			passed++
		} else if kernel.CodeOf(err) == kernel.ErrPreconditionFailed {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if passed != 1 || conflicts != 1 || len(store.rules) != 6 {
		t.Fatalf("editors overwrote or duplicated rules: %d %d %#v", passed, conflicts, store.rules)
	}
}

func TestEditingOwnManagementRuleDoesNotRetainOldAuthority(t *testing.T) {
	service, store := scopedService()
	old := store.rules[1]
	next := old
	next.Actions = []string{"file.read"}
	updated, err := service.Update(context.Background(), "manager", old.ID, old, next)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), "manager", old.ID, updated, old); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("old editor restored revoked management: %v", err)
	}
}
