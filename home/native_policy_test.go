package home

import (
	"kc/kernel"
	"kc/snapshot/access"
	"testing"
)

func TestNativeCreatorAndSharingActionsStayExplicitAndBounded(t *testing.T) {
	for _, action := range []string{access.RepositoryMetadataRead, access.RepositoryFileRead, access.RepositoryHistoryRead, access.RepositoryFilesWrite, access.RepositoryCommit, access.RepositoryRefsManage, access.RepositoryMerge, access.RepositoryDatasetPublish} {
		if err := validateCreatorActions([]string{action}); err != nil {
			t.Fatalf("explicit native creator action rejected: %s %v", action, err)
		}
	}
	for _, action := range []string{access.RepositoryCreate, access.RepositoryImport, access.RepositoryDelete, "admin.grants.manage", "repository.*"} {
		if err := validateCreatorActions([]string{action}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("creator policy expanded: %s %v", action, err)
		}
	}
	if err := validateShareActions([]string{access.RepositoryHistoryRead}); err != nil {
		t.Fatal(err)
	}
	if err := validateShareActions([]string{access.RepositoryFilesWrite}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatal("consume sharing expanded to writes")
	}
	if err := validateShareActions([]string{access.RepositoryDatasetPublish}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatal("ordinary sharing expanded to source publication")
	}
}
