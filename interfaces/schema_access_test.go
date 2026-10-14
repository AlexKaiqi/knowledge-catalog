package interfaces_test

import (
	"context"
	"kc/interfaces"
	interfaceservice "kc/interfaces/service"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"

	"kc/snapshot"
)

func applyInterfaceOperations(t *testing.T, state testkit.Setup, base kernel.CommitID, operations []objectmodel.Operation) kernel.CommitID {
	t.Helper()
	head, err := state.Repo.ApplyKnowledgeCommit(objectmodel.CommitChangeSet{TargetRepository: state.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func interfaceAccessSchema(prefixType string, access []string) map[string]any {
	prefix := map[string]any{"type": prefixType, "required": true}
	if len(access) != 0 {
		prefix["access"] = access
	}
	return map[string]any{"entity": "Object", "aspect": "interfaces", "pattern": "keyed_collection", "additionalProperties": false, "fields": map[string]any{"interfaceRef": map[string]any{"type": "string", "required": true}, "prefix": prefix}}
}

func fixedInterfaceAccessReader(t *testing.T, contracts, state testkit.Setup, contractCommit, commit kernel.CommitID) (*reader.Serving, interfaces.ResolvedInterface) {
	t.Helper()
	repositories := map[kernel.RepositoryID]kernel.CommitID{contracts.RepositoryID: contractCommit, state.RepositoryID: commit}
	base := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id == contracts.RepositoryID {
			return contracts.Repo, nil
		}
		return state.Repo, nil
	}, reader.KnowledgeSetPin{SetID: "interface-access", Repositories: repositories, Items: reader.WholeRepositoryItems(repositories)})
	implementation, err := interfaces.ResolveObjectInterfaceAt(base, state.RepositoryID, "object/message", "friendly")
	if err != nil {
		t.Fatal(err)
	}
	return base, implementation
}

func TestObjectInterfaceAccessEvolutionReusesHandlerAndRetainsFullSchemaBasis(t *testing.T) {
	contracts := testkit.NewSetup(t, "kr://interface/contracts")
	state := testkit.NewSetup(t, "kr://interface/objects")
	contractCommit := applyInterfaceOperations(t, contracts, contracts.RootCommitID, []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "interface/prefix/v1"}, Value: map[string]any{"name": "Prefix", "meaning": "Prefix provided text", "operations": map[string]any{"read": map[string]any{"effect": "read"}}}}})
	contractRef := objectmodel.FormatPinnedRef(contracts.RepositoryID, contractCommit, "interface/prefix/v1")
	schemaAddress := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/implements.prefix/v1"}
	memberAddress := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "object/message", AspectName: interfaces.ObjectInterfacesAspect, MemberKey: "friendly"}
	head := applyInterfaceOperations(t, state, state.RootCommitID, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: schemaAddress, Value: interfaceAccessSchema("string", []string{"text", "filter"})},
		{Op: objectmodel.OpPut, Address: memberAddress, SchemaRef: string(schemaAddress.ObjectID), Value: map[string]any{"interfaceRef": contractRef, "prefix": "Hello "}},
	})
	firstReader, first := fixedInterfaceAccessReader(t, contracts, state, contractCommit, head)
	handler := &prefixInterfaceHandler{}
	registry := registerPrefix(t, first, handler)
	request := interfaces.InterfaceRequest{Repository: state.RepositoryID, Object: memberAddress.ObjectID, Implementation: memberAddress.MemberKey, Operation: "read", Input: map[string]any{"text": "World"}}
	caller := objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}}
	previousDigest := first.ImplementationSchemaDigest
	for _, access := range [][]string{{"filter"}, nil} {
		head = applyInterfaceOperations(t, state, head, []objectmodel.Operation{{Op: objectmodel.OpPut, Address: schemaAddress, Value: interfaceAccessSchema("string", access)}})
		base, current := fixedInterfaceAccessReader(t, contracts, state, contractCommit, head)
		result, err := interfaceservice.Open(base, registry, caller).Call(context.Background(), request)
		if err != nil {
			t.Fatalf("access-only schema change required handler reinstallation: %v", err)
		}
		if result.Value.(map[string]any)["text"] != "Hello World" || result.Basis.Contract != first.Contract || result.Basis.HandlerRevision != "prefix-v1" {
			t.Fatalf("access-only update changed execution: %#v", result)
		}
		if current.ImplementationSchemaStructureDigest != first.ImplementationSchemaStructureDigest || result.Basis.ImplementationSchemaStructureDigest != first.ImplementationSchemaStructureDigest {
			t.Fatalf("field access changed handler support basis: first=%#v current=%#v result=%#v", first, current, result.Basis)
		}
		if current.ImplementationSchemaDigest == previousDigest || result.Basis.ImplementationSchemaDigest != current.ImplementationSchemaDigest || result.Basis.ImplementationSchema.Commit != head {
			t.Fatalf("full schema evidence lost indexing change: previous=%s current=%#v result=%#v", previousDigest, current, result.Basis)
		}
		previousDigest = current.ImplementationSchemaDigest
	}
	old, err := interfaceservice.Open(firstReader, registry, caller).Call(context.Background(), request)
	if err != nil || old.Basis.ImplementationSchemaDigest != first.ImplementationSchemaDigest || old.Basis.ImplementationSchema != first.ImplementationSchema {
		t.Fatalf("historical full schema basis drifted: %#v %v", old, err)
	}
	// A separately published valid implementation keeps the same schema identity
	// and contract, but changes the value type. It must not reuse the handler.
	typed := testkit.NewSetup(t, "kr://interface/typed")
	typedHead := applyInterfaceOperations(t, typed, typed.RootCommitID, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: schemaAddress, Value: interfaceAccessSchema("number", nil)},
		{Op: objectmodel.OpPut, Address: memberAddress, SchemaRef: string(schemaAddress.ObjectID), Value: map[string]any{"interfaceRef": contractRef, "prefix": 7}},
	})
	typedReader, _ := fixedInterfaceAccessReader(t, contracts, typed, contractCommit, typedHead)
	request.Repository = typed.RepositoryID
	_, err = interfaceservice.Open(typedReader, registry, caller).Call(context.Background(), request)
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || handler.calls != 3 {
		t.Fatalf("value type change reused handler: %v calls=%d", err, handler.calls)
	}
}
