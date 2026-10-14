package invocation

import (
	"context"
	interfaces "kc/liveaccess/handler"
	"os"
	"path/filepath"
	"testing"

	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/observability"
)

func interfaceEvidenceFixture() (interfaces.InterfaceCallResult, interfaces.InterfaceRequest, map[string]invoke.FlagValue) {
	request := interfaces.InterfaceRequest{Repository: "kr://demo/object", Object: "table/orders", Implementation: "live", Operation: "read"}
	result := interfaces.InterfaceCallResult{Value: map[string]any{"rows": 12}, Observation: objectmodel.ObservationBasis{BindingGeneration: "orders-v7", Consistency: objectmodel.ObservationBounded, SourceRevision: "source-42", ObservedAt: "2026-10-10T03:00:00Z"}, Basis: interfaces.InterfaceExecutionBasis{DeclarationCommit: "k40", DeclarationDigest: "implementation-digest", Contract: objectmodel.PinnedObjectRef{ObjectRef: objectmodel.ObjectRef{Repository: "kr://demo/contracts", Object: "interface/live-access/v1"}, Commit: "contracts-6"}, ContractDigest: "contract-digest", ImplementationSchema: objectmodel.PinnedObjectRef{ObjectRef: objectmodel.ObjectRef{Repository: request.Repository, Object: "schema/implements.live-access/v1"}, Commit: "k40"}, ImplementationSchemaDigest: "schema-digest", HandlerRevision: "live-access-http/v1"}}
	flags := map[string]invoke.FlagValue{"as": "agent", "_action": "resource.access", "repo": string(request.Repository), "object": string(request.Object), "implementation": request.Implementation, ResolvedCommitFlag: "k40"}
	return result, request, flags
}

func TestInterfaceInvokeRequiresDurableAccessAndAuditEvidence(t *testing.T) {
	home := t.TempDir()
	result, request, flags := interfaceEvidenceFixture()
	output, err := EvidenceChain(context.Background(), nil, home, "object-invoke", flags, ObserveObjectInterfaceCall(result, request), nil)
	if err != nil || output == nil {
		t.Fatalf("evidence chain: %#v %v", output, err)
	}
	page, err := observability.NewFileStore(home).Access(context.Background(), observability.AccessQuery{})
	if err != nil || len(page.Entries) != 1 {
		t.Fatalf("interface invoke emitted no durable access: %#v %v", page, err)
	}
	trail, err := ReadTrail(home, "", "", 1)
	if err != nil || len(trail) != 1 || trail[0].EvidenceID != page.Entries[0].EvidenceID {
		t.Fatalf("audit did not retain access receipt: %#v %v", trail, err)
	}
	refs := trail[0].Refs
	if refs["handlerRevision"] != "live-access-http/v1" || refs["contractDigest"] != "contract-digest" || refs["implementationSchemaDigest"] != "schema-digest" {
		t.Fatalf("audit dropped interface execution basis: %#v", refs)
	}
}

func TestLegacyResourceInvokeAccessEvidenceAndPersistenceFailure(t *testing.T) {
	home := t.TempDir()
	_, _, flags := interfaceEvidenceFixture()
	id, _, err := RecordObjectAccess(home, "object-invoke", flags, map[string]any{"arbitrary": "source result"}, nil)
	if err != nil || id == "" {
		t.Fatalf("legacy invoke omitted accessed descriptor: %q %v", id, err)
	}
	blocked := t.TempDir()
	if err := os.WriteFile(filepath.Join(blocked, "access"), []byte("blocked partition"), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := EvidenceChain(context.Background(), nil, blocked, "object-invoke", flags, map[string]any{"arbitrary": "source result"}, nil)
	if err == nil || output != nil {
		t.Fatalf("invoke returned without durable evidence: %#v %v", output, err)
	}
}

func TestInterfaceReadEvidenceUsesExistingMemberObservation(t *testing.T) {
	result, request, _ := interfaceEvidenceFixture()
	// A typed result must be projected explicitly into the existing evidence
	// envelope; opaque business content must not be treated as object refs.
	accesses := ObjectAccesses(ObserveObjectInterfaceCall(result, request))
	if len(accesses) != 3 {
		t.Fatalf("interface declaration/contract/schema evidence missing: %#v", accesses)
	}
	if accesses[0].Address == nil || accesses[0].Address.MemberKey != request.Implementation || len(accesses[0].Observations) != 1 || accesses[0].Observations[0].Basis.SourceRevision != "source-42" || accesses[1].ObjectRef != result.Basis.Contract || accesses[2].ObjectRef != result.Basis.ImplementationSchema {
		t.Fatalf("fixed object or observation basis lost: %#v", accesses)
	}
	result.Value = map[string]any{"objectRef": map[string]any{"repository": "kr://opaque/source", "commit": "fake", "object": "fake/object"}}
	result.Observation = map[string]any{"unrelated": "custom extension basis"}
	accesses = ObjectAccesses(ObserveObjectInterfaceCall(result, request))
	if len(accesses) != 3 || len(accesses[0].Observations) != 0 {
		t.Fatalf("opaque extension values were promoted to object or state observation: %#v", accesses)
	}
}

func TestFailedInterfaceInvokeEvidenceNamesRequestedMember(t *testing.T) {
	_, request, flags := interfaceEvidenceFixture()
	home := t.TempDir()
	_, _, err := RecordObjectAccess(home, "object-invoke", flags, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "handler unavailable"))
	if err != nil {
		t.Fatal(err)
	}
	page, err := observability.NewFileStore(home).Access(context.Background(), observability.AccessQuery{})
	if err != nil || len(page.Entries) != 1 || len(page.Entries[0].Object) != 1 {
		t.Fatalf("failed invoke evidence: %#v %v", page, err)
	}
	address := page.Entries[0].Object[0].Address
	if page.Entries[0].Result != "ERROR" || address == nil || address.Kind != objectmodel.KindMember || address.AspectName != interfaces.ObjectInterfacesAspect || address.MemberKey != request.Implementation {
		t.Fatalf("failed interface call was recorded as a different unit: %#v", page.Entries[0])
	}
}
