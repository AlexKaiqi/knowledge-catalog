package serving_test

import (
	"sync"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/serving"
)

func publishedLiveImplementation(t *testing.T) reader.ResolvedInterface {
	t.Helper()
	live := reader.ResolvedInterface{Repository: "kr://test/live", Commit: "live-7", Address: knowledge.Address{Kind: knowledge.KindMember, ObjectID: "table/orders", AspectName: knowledge.ObjectInterfacesAspect, MemberKey: "live"}, Contract: knowledge.PinnedKnowledgeRef{KnowledgeRef: knowledge.KnowledgeRef{Repository: "kr://test/live", Object: knowledge.LiveAccessInterfaceV1}, Commit: "live-7"}, ImplementationSchema: knowledge.PinnedKnowledgeRef{KnowledgeRef: knowledge.KnowledgeRef{Repository: "kr://test/live", Object: knowledge.LiveAccessImplementationSchemaV1}, Commit: "live-7"}}
	for _, operation := range knowledge.SystemPublicationOperations() {
		switch operation.Address.ObjectID {
		case knowledge.LiveAccessInterfaceV1:
			definition, err := knowledge.ParseInterfaceDefinition(operation.Address.ObjectID, operation.Value)
			if err != nil {
				t.Fatal(err)
			}
			live.Definition = definition
			live.ContractDigest = definition.Digest
		case knowledge.LiveAccessImplementationSchemaV1:
			schema, err := knowledge.ParseSchemaDefinition(operation.Address.ObjectID, operation.Value)
			if err != nil {
				t.Fatal(err)
			}
			live.ImplementationSchemaDigest = kernel.CanonicalDigest(schema)
			live.ImplementationSchemaStructureDigest = knowledge.SchemaStructureDigest(schema)
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
			composed, err := serving.WithLiveAccess(installed, live, &liveAccessor{})
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
	if err := serving.RegisterLiveAccess(installed, live, &liveAccessor{}); err != nil {
		t.Fatalf("request mutated shared registrations: %v", err)
	}
	reused, err := serving.WithLiveAccess(installed, live, nil)
	if err != nil || reused != installed {
		t.Fatalf("installed handler was not reused: %v", err)
	}
	unsupported := extension
	unsupported.ContractDigest = "unsupported-contract"
	if _, err := serving.WithLiveAccess(installed, unsupported, &liveAccessor{}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unknown custom release gained built-in fallback: %v", err)
	}
}

func TestLiveAccessTrustedPublicationMatchesStructureAndPreservesFullSchemaDigest(t *testing.T) {
	live := publishedLiveImplementation(t)
	var definition knowledge.SchemaDefinition
	for _, operation := range knowledge.SystemPublicationOperations() {
		if operation.Address.ObjectID == knowledge.LiveAccessImplementationSchemaV1 {
			var err error
			definition, err = knowledge.ParseSchemaDefinition(operation.Address.ObjectID, operation.Value)
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
	withoutAccess.ImplementationSchemaStructureDigest = knowledge.SchemaStructureDigest(definition)
	if withoutAccess.ImplementationSchemaDigest == live.ImplementationSchemaDigest {
		t.Fatal("published schema must exercise an access declaration")
	}
	if err := serving.RegisterLiveAccess(serving.NewInterfaceRegistry(), withoutAccess, &liveAccessor{}); err != nil {
		t.Fatalf("compatible historical schema required an access declaration: %v", err)
	}
	if withoutAccess.ImplementationSchemaDigest != kernel.CanonicalDigest(definition) || live.ImplementationSchemaDigest == withoutAccess.ImplementationSchemaDigest {
		t.Fatal("trusted registration replaced the complete historical schema digest")
	}
	definition.Fields[0].Required = !definition.Fields[0].Required
	changed := withoutAccess
	changed.ImplementationSchemaDigest = kernel.CanonicalDigest(definition)
	changed.ImplementationSchemaStructureDigest = knowledge.SchemaStructureDigest(definition)
	if err := serving.RegisterLiveAccess(serving.NewInterfaceRegistry(), changed, &liveAccessor{}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("value structure change passed trusted publication matching: %v", err)
	}
}
