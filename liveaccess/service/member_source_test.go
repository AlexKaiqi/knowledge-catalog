package service_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess/evidence"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type memberPrivateSchemaGuard struct {
	snapshot.Store
	snapshot.TreeStore
	schemaReads int
}

func (g *memberPrivateSchemaGuard) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	if path == "private/schema.yaml" {
		g.schemaReads++
		return nil, fmt.Errorf("unpublished Schema body must not be read")
	}
	return g.TreeStore.ReadFile(path, commit)
}

func memberSourceOperations(bound bool) []objectmodel.Operation {
	schema := map[string]any{"entity": "Widget", "aspect": "readings", "pattern": "keyed_collection",
		"fields": map[string]any{"value": map[string]any{"type": "number"}}}
	address := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "widget/selected", AspectName: "readings", MemberKey: "good"}
	member := objectmodel.Operation{Op: objectmodel.OpPut, Address: address, SchemaRef: "schema/widget.readings", PathHint: "public/member.yaml", Value: map[string]any{"value": float64(31)}}
	operations := []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/widget.readings"}, PathHint: "private/schema.yaml", Value: schema},
	}
	if bound {
		descriptor := bindingfixture.PutDescriptor(widgetDescriptor, bindingfixture.Descriptor("https://source.example"))
		descriptor.PathHint = "private/descriptor.yaml"
		member = bindingfixture.Bind(address, "schema/widget.readings", widgetDescriptor)
		member.PathHint = "public/member.yaml"
		operations = append(operations, descriptor)
	}
	return append(operations, member)
}

func selectedMemberScope(repo objectmodel.Repository, commit kernel.CommitID) *reader.Serving {
	return reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }, reader.DatasetPin{
		Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit},
		Items:        []reader.DatasetItem{{Repository: repo.ID(), Commit: commit, Kind: "prefix", Prefix: "public"}},
	})
}

func TestMemberSourceCategoryDoesNotRequireUnpublishedSchemaBody(t *testing.T) {
	for _, bound := range []bool{false, true} {
		t.Run(fmt.Sprint("bound=", bound), func(t *testing.T) {
			var guard *memberPrivateSchemaGuard
			_, repo, commit, _ := publishUnitFixture(t, "widget/selected", memberSourceOperations(bound), func(raw snapshot.Store) snapshot.Store {
				guard = &memberPrivateSchemaGuard{Store: raw, TreeStore: raw.(snapshot.TreeStore)}
				return guard
			})
			assertScopedMemberCategory(t, selectedMemberScope(repo, commit), bound)
			if guard.schemaReads != 0 {
				t.Fatalf("classification or access read the unpublished Schema body: %d", guard.schemaReads)
			}
		})
	}
}

func assertScopedMemberCategory(t *testing.T, scope *reader.Serving, bound bool) {
	t.Helper()
	address := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "widget/selected", AspectName: "readings", MemberKey: "good"}
	collector := evidence.NewCollector()
	lookup := &unitLookup{}
	ctx := evidence.WithCollector(context.Background(), collector)
	service := liveservice.Open(scope, lookup, kernel.IdentityContext{Principal: "consumer"})
	values, err := service.ReadAddress(ctx, address)
	if err != nil || len(values) != 1 || len(lookup.requests) != 0 {
		t.Fatalf("fixed source classification changed read scope: %#v requests=%#v error=%v", values, lookup.requests, err)
	}
	if bound {
		assertUnitFailure(t, values[0], address, kernel.ErrObjectRefUnresolved)
		samples := collectedSamples(t, collector)
		if len(samples) != 1 || samples[0].Outcome != evidence.FailureOutcome || samples[0].Observation != nil ||
			samples[0].Declaration.ValueSource == nil || samples[0].Declaration.ValueSource.Kind != objectmodel.ValueSourceBinding {
			t.Fatalf("unavailable Binding became a fixed placeholder or lost its declared category: %#v", samples)
		}
		return
	}
	if !reflect.DeepEqual(values[0].Value, map[string]any{"value": float64(31)}) || len(values[0].Failures) != 0 ||
		len(values[0].Observations) != 0 || len(collectedSamples(t, collector)) != 0 || values[0].Declarations[0].ValueSource.Normalized() != nil {
		t.Fatalf("ordinary Snapshot Member acquired a LiveAccess failure or audit payload: %#v", values)
	}
}

type nativeMemberSourceFixture struct {
	*testkit.ObjectRepository
	bound       bool
	schemaReads int
}

func (*nativeMemberSourceFixture) NativeObjectRepository() {}

func (n *nativeMemberSourceFixture) Read(id objectmodel.ObjectID, commit kernel.CommitID) (objectmodel.ObjectValue, error) {
	if objectmodel.IsSchemaObject(id) {
		n.schemaReads++
		return objectmodel.ObjectValue{}, fmt.Errorf("unpublished native Schema must not be read")
	}
	value, err := n.ObjectRepository.Read(id, commit)
	return n.declare(value), err
}

func (n *nativeMemberSourceFixture) ReadAddress(address objectmodel.Address, commit kernel.CommitID) (objectmodel.ObjectValue, error) {
	value, err := n.ObjectRepository.ReadAddress(address, commit)
	return n.declare(value), err
}

func (n *nativeMemberSourceFixture) declare(value objectmodel.ObjectValue) objectmodel.ObjectValue {
	if n.bound {
		for i := range value.Declarations {
			value.Declarations[i].ValueSource = bindingfixture.Source(widgetDescriptor)
			value.Declarations[i].DeclarationDigest = objectmodel.DeclarationDigest(value.Declarations[i].SchemaRef, value.Declarations[i].ValueSource)
		}
	}
	return value
}

func TestNativeMemberKeepsDefaultSnapshotAndExplicitBindingCategories(t *testing.T) {
	for _, bound := range []bool{false, true} {
		t.Run(fmt.Sprint("bound=", bound), func(t *testing.T) {
			s := testkit.NewSetup(t, "kr://native/member-source")
			receipt, err := s.Writer.Commit("native-member-source", objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
				BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: memberSourceOperations(bound)})
			if err != nil {
				t.Fatal(err)
			}
			native := &nativeMemberSourceFixture{ObjectRepository: s.Repo, bound: bound}
			repo, err := reader.NewReader(nil).Wrap(native, kernel.ErrCapabilityUnsatisfied)
			if err != nil {
				t.Fatal(err)
			}
			assertScopedMemberCategory(t, selectedMemberScope(repo, receipt.Result.NewCommit), bound)
			if native.schemaReads != 0 {
				t.Fatalf("native read escaped Schema publication: %d", native.schemaReads)
			}
		})
	}
}
