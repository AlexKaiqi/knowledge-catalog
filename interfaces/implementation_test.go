package interfaces_test

import (
	"kc/interfaces"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func commitInterfaceFixture(t *testing.T, s testkit.Setup, operations []objectmodel.Operation) kernel.CommitID {
	t.Helper()
	head, err := s.Repo.ApplyKnowledgeCommit(objectmodel.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func TestObjectInterfaceExplicitForeignContractRemainsPinnedAndScoped(t *testing.T) {
	contracts := testkit.NewSetup(t, "kr://demo/contracts")
	business := testkit.NewSetup(t, "kr://demo/business")
	contractCommit := commitInterfaceFixture(t, contracts, []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "interface/custom/v1"}, Value: map[string]any{"name": "Custom", "meaning": "Domain extension", "operations": map[string]any{"read": map[string]any{"effect": "read"}}}}})
	implementation := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "object/example", AspectName: interfaces.ObjectInterfacesAspect, MemberKey: "custom"}
	businessCommit := commitInterfaceFixture(t, business, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/custom/v1"}, Value: map[string]any{"entity": "Object", "aspect": "interfaces", "pattern": "keyed_collection", "fields": map[string]any{"interfaceRef": map[string]any{"type": "string", "required": true}}}},
		{Op: objectmodel.OpPut, Address: implementation, SchemaRef: "schema/custom/v1", Value: map[string]any{"interfaceRef": objectmodel.FormatPinnedRef(contracts.RepositoryID, contractCommit, "interface/custom/v1")}},
	})
	repos := map[kernel.RepositoryID]objectmodel.Repository{contracts.RepositoryID: contracts.Repo, business.RepositoryID: business.Repo}
	pin := reader.KnowledgeSetPin{SetID: "both", Repositories: map[kernel.RepositoryID]kernel.CommitID{contracts.RepositoryID: contractCommit, business.RepositoryID: businessCommit}}
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	lookups := map[kernel.RepositoryID]int{}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) { lookups[id]++; return repos[id], nil }
	serving := reader.Open(lookup, pin)
	resolved, err := interfaces.ResolveObjectInterfaceAt(serving, business.RepositoryID, implementation.ObjectID, "custom")
	if err != nil || resolved.Contract.Repository != contracts.RepositoryID || resolved.Contract.Commit != contractCommit {
		t.Fatalf("cross repo fixed contract: %#v %v", resolved, err)
	}
	_, err = interfaces.ResolveRepoObjectInterface(business.Repo, businessCommit, implementation.ObjectID, "custom")
	if kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		t.Fatalf("repository reader inferred foreign dependency: %v", err)
	}
	before := lookups[contracts.RepositoryID]
	delete(pin.Repositories, contracts.RepositoryID)
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	_, err = interfaces.ResolveObjectInterfaceAt(reader.Open(lookup, pin), business.RepositoryID, implementation.ObjectID, "custom")
	if kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved || lookups[contracts.RepositoryID] != before {
		t.Fatalf("cross-scope dependency lookup: %v lookups %#v", err, lookups)
	}
}
