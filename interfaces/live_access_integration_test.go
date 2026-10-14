package interfaces_test

import (
	"context"
	"kc/interfaces"
	liveaccesscontract "kc/liveaccess/contract"
	"kc/systemrepo"
	"sync"
	"testing"

	"kc/kernel"
	serving "kc/liveaccess"
	objectmodel "kc/object"
)

func publishedLiveImplementation(t *testing.T) interfaces.ResolvedInterface {
	t.Helper()
	live := interfaces.ResolvedInterface{Repository: "kr://test/live", Commit: "live-7", Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: interfaces.ObjectInterfacesAspect, MemberKey: "live"}, Contract: objectmodel.PinnedKnowledgeRef{KnowledgeRef: objectmodel.KnowledgeRef{Repository: "kr://test/live", Object: liveaccesscontract.LiveAccessInterfaceV1}, Commit: "live-7"}, ImplementationSchema: objectmodel.PinnedKnowledgeRef{KnowledgeRef: objectmodel.KnowledgeRef{Repository: "kr://test/live", Object: liveaccesscontract.LiveAccessImplementationSchemaV1}, Commit: "live-7"}}
	for _, operation := range systemrepo.SystemPublicationOperations() {
		switch operation.Address.ObjectID {
		case liveaccesscontract.LiveAccessInterfaceV1:
			definition, err := interfaces.ParseInterfaceDefinition(operation.Address.ObjectID, operation.Value)
			if err != nil {
				t.Fatal(err)
			}
			live.Definition = definition
			live.ContractDigest = definition.Digest
		case liveaccesscontract.LiveAccessImplementationSchemaV1:
			schema, err := objectmodel.ParseSchemaDefinition(operation.Address.ObjectID, operation.Value)
			if err != nil {
				t.Fatal(err)
			}
			live.ImplementationSchemaDigest = kernel.CanonicalDigest(schema)
			live.ImplementationSchemaStructureDigest = objectmodel.SchemaStructureDigest(schema)
		}
	}
	if live.ContractDigest == "" || live.ImplementationSchemaDigest == "" || live.ImplementationSchemaStructureDigest == "" {
		t.Fatal("published LiveAccess basis missing")
	}
	return live
}

func TestLiveAccessCompositionPreservesSharedExtensionsUnderConcurrency(t *testing.T) {
	_, extension := interfaceFixture(t)
	installed := registerPrefix(t, extension, &prefixInterfaceHandler{})
	live := publishedLiveImplementation(t)
	var workers sync.WaitGroup
	for range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			composed, err := serving.WithLiveAccess(installed, live, &integrationAccessor{})
			if err != nil || composed == installed {
				t.Errorf("built-in handler did not use request-local composition: %v", err)
				return
			}
			reused, err := serving.WithLiveAccess(composed, extension, nil)
			if err != nil || reused != composed {
				t.Errorf("composition lost installed extension: %v", err)
			}
		}()
	}
	workers.Wait()
	// This registration succeeds only if parallel compositions never mutated
	// the shared registry by installing the selected LiveAccess release there.
	if err := serving.RegisterLiveAccess(installed, live, &integrationAccessor{}); err != nil {
		t.Fatalf("request mutated shared registrations: %v", err)
	}
	reused, err := serving.WithLiveAccess(installed, live, nil)
	if err != nil || reused != installed {
		t.Fatalf("installed handler was not reused: %v", err)
	}
	unsupported := extension
	unsupported.ContractDigest = "unsupported-contract"
	if _, err := serving.WithLiveAccess(installed, unsupported, &integrationAccessor{}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unknown custom release gained built-in fallback: %v", err)
	}
}

func TestLiveAccessTrustedPublicationMatchesStructureAndPreservesFullSchemaDigest(t *testing.T) {
	live := publishedLiveImplementation(t)
	var definition objectmodel.SchemaDefinition
	for _, operation := range systemrepo.SystemPublicationOperations() {
		if operation.Address.ObjectID == liveaccesscontract.LiveAccessImplementationSchemaV1 {
			var err error
			definition, err = objectmodel.ParseSchemaDefinition(operation.Address.ObjectID, operation.Value)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	for i := range definition.Fields {
		definition.Fields[i].Access = nil
	}
	withoutAccess := live
	withoutAccess.ImplementationSchemaDigest = kernel.CanonicalDigest(definition)
	withoutAccess.ImplementationSchemaStructureDigest = objectmodel.SchemaStructureDigest(definition)
	if withoutAccess.ImplementationSchemaDigest == live.ImplementationSchemaDigest {
		t.Fatal("published schema must exercise an access declaration")
	}
	if err := serving.RegisterLiveAccess(interfaces.NewInterfaceRegistry(), withoutAccess, &integrationAccessor{}); err != nil {
		t.Fatalf("compatible historical schema required an access declaration: %v", err)
	}
	if withoutAccess.ImplementationSchemaDigest != kernel.CanonicalDigest(definition) || live.ImplementationSchemaDigest == withoutAccess.ImplementationSchemaDigest {
		t.Fatal("trusted registration replaced the complete historical schema digest")
	}
	definition.Fields[0].Required = !definition.Fields[0].Required
	changed := withoutAccess
	changed.ImplementationSchemaDigest = kernel.CanonicalDigest(definition)
	changed.ImplementationSchemaStructureDigest = objectmodel.SchemaStructureDigest(definition)
	if err := serving.RegisterLiveAccess(interfaces.NewInterfaceRegistry(), changed, &integrationAccessor{}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("value structure change passed trusted publication matching: %v", err)
	}
}

type integrationAccessor struct {
	value any
	calls int
}

func (a *integrationAccessor) AccessResource(_ context.Context, _ serving.ResourceOperationRequest) (any, error) {
	a.calls++
	return a.value, nil
}
