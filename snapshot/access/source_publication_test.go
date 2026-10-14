package access_test

import (
	"context"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot/access"
)

const sourcePublicationAction = "repository.dataset.publish"

func TestDatasetOnlyGrantManagerCannotAcquireSourcePublication(t *testing.T) {
	for _, operation := range []string{"add", "add-repository-wildcard", "update-to-publication", "update-from-publication", "revoke"} {
		t.Run(operation, func(t *testing.T) {
			manager := access.Rule{ID: "dataset-admin", Principal: "editor", Dataset: "files", Actions: []string{"admin.grants.manage"}}
			original := access.Rule{ID: "target", Principal: "editor", Repo: "private", Dataset: "files", Ref: "refs/heads/main", Actions: []string{sourcePublicationAction}}
			requested := original
			if operation == "update-to-publication" {
				original.Actions = []string{"file.read"}
			}
			if operation == "update-from-publication" {
				requested.Actions = []string{"file.read"}
			}
			if operation == "add-repository-wildcard" {
				requested.Actions = []string{"repository.*"}
			}
			store := &memoryRules{rules: []access.Rule{manager, original, {ID: "repository-reader", Principal: "editor", Repo: "private", Actions: []string{"file.read"}}}}
			before := cloneRules(store.rules)
			service := access.NewService(store)
			var err error
			switch operation {
			case "add", "add-repository-wildcard":
				_, err = service.Add(context.Background(), "editor", requested)
			case "update-to-publication", "update-from-publication":
				_, err = service.Update(context.Background(), "editor", original.ID, original, requested)
			case "revoke":
				err = service.Revoke(context.Background(), "editor", original.ID)
			}
			if kernel.CodeOf(err) != kernel.ErrForbidden {
				t.Errorf("Dataset-only administrator acquired source authority via %s: %v", operation, err)
			}
			if !reflect.DeepEqual(store.rules, before) {
				t.Errorf("rejected source administration changed policy: before=%+v after=%+v", before, store.rules)
			}
		})
	}
}

func TestSourcePublicationGrantManagementRequiresRepositoryOrGlobalScope(t *testing.T) {
	for _, scope := range []struct {
		name    string
		repo    string
		dataset string
	}{{"global", "", ""}, {"repository", "private", ""}, {"intersection", "private", "files"}} {
		t.Run(scope.name, func(t *testing.T) {
			store := &memoryRules{rules: []access.Rule{{ID: "source-admin", Principal: "owner", Repo: scope.repo, Dataset: scope.dataset, Actions: []string{"admin.grants.manage"}}}}
			service := access.NewService(store)
			requested := access.Rule{Principal: "publisher", Repo: "private", Dataset: "files", Ref: "refs/heads/main", Actions: []string{sourcePublicationAction}}
			granted, err := service.Add(context.Background(), "owner", requested)
			if err != nil || granted.ID == "" || granted.Repo != requested.Repo || granted.Dataset != requested.Dataset || granted.Ref != requested.Ref {
				t.Fatalf("explicit source manager could not grant its scope: %+v, %v", granted, err)
			}
			if err := service.Revoke(context.Background(), "owner", granted.ID); err != nil || len(store.rules) != 1 {
				t.Fatalf("source manager could not revoke its own scoped grant: %v, %+v", err, store.rules)
			}
		})
	}
}

func TestSourcePublicationMatcherKeepsRepositoryDestinationAndRefSeparate(t *testing.T) {
	query := access.Query{Principal: "publisher", Action: sourcePublicationAction, Repo: "private", Dataset: "files", Ref: "refs/heads/main"}
	for _, tc := range []struct {
		name    string
		rule    access.Rule
		allowed bool
	}{
		{"repository", access.Rule{Repo: "private"}, true},
		{"intersection", access.Rule{Repo: "private", Dataset: "files", Ref: "refs/heads/main"}, true},
		{"global", access.Rule{}, true},
		{"dataset-only", access.Rule{Dataset: "files"}, false},
		{"other-repository", access.Rule{Repo: "other", Dataset: "files"}, false},
		{"other-destination", access.Rule{Repo: "private", Dataset: "other"}, false},
		{"other-ref", access.Rule{Repo: "private", Ref: "c2"}, false},
		{"object-is-not-file-acl", access.Rule{Repo: "private", Object: "*"}, false},
		{"aspect-is-not-file-acl", access.Rule{Repo: "private", Aspect: "canonical"}, false},
		{"ordinary-read", access.Rule{Repo: "private", Actions: []string{"file.read"}}, false},
		{"dataset-wildcard", access.Rule{Repo: "private", Dataset: "files", Actions: []string{"dataset.*"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.rule.Principal = "publisher"
			if tc.rule.Actions == nil {
				tc.rule.Actions = []string{sourcePublicationAction}
			}
			if _, allowed := access.MatchSourcePublication([]access.Rule{tc.rule}, query); allowed != tc.allowed {
				t.Fatalf("source publication scope changed: rule=%+v query=%+v allowed=%v", tc.rule, query, allowed)
			}
		})
	}
	if !access.IsRepositoryCreatorAction(access.RepositoryDatasetPublish) || access.IsRepositoryReadAction(access.RepositoryDatasetPublish) {
		t.Fatal("source publication did not remain an explicit creator choice outside ordinary sharing reads")
	}
}
