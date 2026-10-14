package invocation

import (
	"testing"

	"kc/kernel"
	"kc/object/reader"
)

func TestWorkspaceRelationEndpointRequiresUnpinnedMemberObjectRef(t *testing.T) {
	pin := reader.DatasetPin{SetID: "w", Repositories: map[kernel.RepositoryID]kernel.CommitID{
		"kr://acme/public/core": "c1",
	}}
	ref, err := WorkspaceRelationEndpoint("kc://acme/public/core/Table:orders", pin)
	if err != nil || ref.Repository != "kr://acme/public/core" || ref.Object != "Table:orders" {
		t.Fatalf("ref=%#v err=%v", ref, err)
	}
	for _, invalid := range []string{
		"Table:orders",
		"kc://acme/public/core@c1/Table:orders",
		"kc://acme/private/core/Table:orders",
	} {
		if _, err := WorkspaceRelationEndpoint(invalid, pin); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("%q: got %v, want USAGE_INVALID", invalid, err)
		}
	}
}
