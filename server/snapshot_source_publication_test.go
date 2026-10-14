package server

import (
	"context"
	"testing"

	"kc/identity"
	inv "kc/invocation"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	snapshotservice "kc/snapshot/service"
)

func TestSnapshotHostSourcePublicationUsesTheSharedSourceScope(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rule    access.Rule
		allowed bool
	}{
		{"explicit", access.Rule{Repo: "private", Dataset: "files", Ref: snapshot.DefaultRef, Actions: []string{access.RepositoryDatasetPublish}}, true},
		{"source-read", access.Rule{Repo: "private", Actions: []string{"file.read"}}, false},
		{"dataset-manage", access.Rule{Dataset: "files", Actions: []string{"dataset.manage"}}, false},
		{"dataset-only-source-permit", access.Rule{Dataset: "files", Actions: []string{access.RepositoryDatasetPublish}}, false},
		{"other-ref", access.Rule{Repo: "private", Dataset: "files", Ref: "c2", Actions: []string{access.RepositoryDatasetPublish}}, false},
		{"other-destination", access.Rule{Repo: "private", Dataset: "other", Actions: []string{access.RepositoryDatasetPublish}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.rule.Principal = "publisher"
			if err := inv.WriteAllow(dir, inv.AllowFile{Rules: []access.Rule{tc.rule}}); err != nil {
				t.Fatal(err)
			}
			facade := &httpFacade{}
			opened := &Home{Dir: dir}
			request := snapshotservice.Authorization{Caller: identity.HTTPIdentity{Principal: "publisher"}, Query: access.Query{Action: access.RepositoryDatasetPublish, Repo: "private", Dataset: "files", Ref: snapshot.DefaultRef}}
			err := facade.authorizeSnapshotRequest(context.Background(), opened, "dataset-define", false, request)
			if tc.allowed {
				if err != nil {
					t.Fatalf("host rejected explicit source selection: %v", err)
				}
				if err := inv.WriteAllow(dir, inv.AllowFile{}); err != nil {
					t.Fatal(err)
				}
				err = facade.authorizeSnapshotRequest(context.Background(), opened, "dataset-define", false, request)
			}
			if kernel.CodeOf(err) != kernel.ErrForbidden {
				t.Fatalf("host policy inferred or cached source authority: %v", err)
			}
		})
	}
}
