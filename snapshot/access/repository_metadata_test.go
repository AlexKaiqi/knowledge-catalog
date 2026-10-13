package access

import "testing"

func TestRepositoryMetadataActionsHaveBoundedPolicyRoles(t *testing.T) {
	for _, action := range []string{RepositoryMetadataRead, RepositoryMetadataWrite} {
		if !IsRepositoryAction(action) || !IsRepositoryCreatorAction(action) {
			t.Fatalf("explicit metadata action is unavailable: %s", action)
		}
	}
	if !IsRepositoryReadAction(RepositoryMetadataRead) || IsRepositoryReadAction(RepositoryMetadataWrite) {
		t.Fatal("metadata write entered consume-only sharing")
	}
}
