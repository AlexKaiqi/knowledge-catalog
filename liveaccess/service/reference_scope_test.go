package service_test

import (
	"fmt"
	"strings"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

type logicalReferenceStoreGuard struct {
	snapshot.Store
	snapshot.TreeStore
	reads map[string]int
}

func (r *logicalReferenceStoreGuard) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	r.reads[path]++
	if strings.HasPrefix(path, "private/") {
		return nil, fmt.Errorf("unpublished body accessed: %s", path)
	}
	return r.TreeStore.ReadFile(path, at)
}
func (*logicalReferenceStoreGuard) ListFiles(kernel.CommitID) ([]string, error) {
	return nil, fmt.Errorf("logical Reference must not scan files")
}

func TestLogicalBindingReferenceRequiresPublishedUnitSchemaAndDescriptor(t *testing.T) {
	const private = "private/"
	for _, tc := range []struct {
		name                                 string
		unitPath, schemaPath, descriptorPath string
		want                                 kernel.ErrorCode
		unitFailure                          bool
	}{
		{"published-evidence", "public/runtime.yaml", "public/schema.yaml", "public/descriptor.yaml", "", false},
		{"unpublished-unit", private + "runtime.yaml", "public/schema.yaml", "public/descriptor.yaml", kernel.ErrObjectRefUnresolved, false},
		{"unpublished-schema", "public/runtime.yaml", private + "schema.yaml", "public/descriptor.yaml", kernel.ErrObjectRefUnresolved, true},
		{"unpublished-descriptor", "public/runtime.yaml", "public/schema.yaml", private + "descriptor.yaml", kernel.ErrObjectRefUnresolved, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := testkit.MakeTreeStore(t, "kr://logical/scoped")
			registry := snapshot.NewRegistry()
			if err := registry.Add(raw); err != nil {
				t.Fatal(err)
			}
			w, err := writer.NewWriter(registry, nil)
			if err != nil {
				t.Fatal(err)
			}
			root := testkit.MustHead(t, raw, snapshot.DefaultRef)
			descriptor := bindingfixture.PutDescriptor("resource/runtime", bindingfixture.Descriptor("https://runtime.example"))
			descriptor.PathHint = tc.descriptorPath
			runtimeUnit := bindingfixture.PutAspect("job/scoped", "runtime", "schema/job.runtime", "resource/runtime")
			runtimeUnit.PathHint = tc.unitPath
			receipt, err := w.Commit("scoped-reference", objectmodel.ChangeSet{TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root, Operations: []objectmodel.Operation{
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, PathHint: tc.schemaPath, Value: map[string]any{"entity": "Job", "aspect": "runtime", "fields": map[string]any{"status": map[string]any{"type": "string"}}}},
				descriptor, runtimeUnit,
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/scoped", AspectName: "definition"}, PathHint: "public/definition.yaml", Value: map[string]any{"owner": "agent"}},
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/scoped", AspectName: "audit"}, PathHint: private + "sibling.yaml", Value: map[string]any{"rule": "fixed"}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			guard := &logicalReferenceStoreGuard{Store: raw, TreeStore: raw.(snapshot.TreeStore), reads: map[string]int{}}
			readRegistry := snapshot.NewRegistry()
			if err := readRegistry.Add(guard); err != nil {
				t.Fatal(err)
			}
			rd := reader.NewReader(readRegistry)
			at := receipt.Result.NewCommit
			scope := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
				return rd.Require(id, kernel.ErrObjectRefUnresolved)
			}, reader.DatasetPin{SetID: "scoped", Repositories: map[kernel.RepositoryID]kernel.CommitID{raw.ID(): at}, Items: []reader.DatasetItem{{Repository: raw.ID(), Commit: at, Kind: "prefix", Prefix: "public"}}})
			lookup := &stateLookup{result: liveaccess.StateObservation{Value: map[string]any{"status": "running"}, Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "rev-1", ObservedAt: "2026-10-13T00:00:00Z"}}}
			address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/scoped", AspectName: "runtime"}
			values, err := readLogicalReference(t, liveservice.Open(scope, lookup, kernel.IdentityContext{Principal: "agent"}), objectmodel.Reference{Repository: raw.ID(), Object: "job/scoped", Locator: "/runtime/status"})
			switch {
			case tc.unitFailure:
				assertFailedUnitRead(t, values, err, address, tc.want)
			case tc.want != "":
				testkit.ExpectCode(t, err, tc.want)
			case err != nil || len(values) != 1 || values[0].Value != "running" || len(lookup.requests) != 1 || lookup.requests[0].Binding.DeclarationCommit != at:
				t.Fatalf("permitted Binding evidence: %#v %#v %v", values, lookup.requests, err)
			}
			if tc.want != "" && len(lookup.requests) != 0 {
				t.Fatalf("unpublished evidence invoked runtime: %#v", lookup.requests)
			}
			for path, n := range guard.reads {
				if n != 0 && strings.HasPrefix(path, private) {
					t.Fatalf("auxiliary read escaped Dataset scope: %#v", guard.reads)
				}
			}
		})
	}
}
