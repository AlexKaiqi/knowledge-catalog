package service_test

import (
	"fmt"
	"testing"

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
	if path == "private/schema.yaml" || path == "private/sibling.yaml" {
		return nil, fmt.Errorf("unpublished body accessed: %s", path)
	}
	return r.TreeStore.ReadFile(path, at)
}
func (*logicalReferenceStoreGuard) ListFiles(kernel.CommitID) ([]string, error) {
	return nil, fmt.Errorf("logical Reference must not scan files")
}

func TestLogicalVirtualBoundReferenceRequiresPublishedObjectAndSchemaEvidence(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		schemaPath, siblingPath string
		wantErr                 bool
	}{
		{"published-evidence", "public/schema.yaml", "public/sibling.yaml", false},
		{"unpublished-object-evidence", "public/schema.yaml", "private/sibling.yaml", true},
		{"unpublished-schema-evidence", "private/schema.yaml", "public/sibling.yaml", true},
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
			receipt, err := w.Commit("scoped-reference", objectmodel.ChangeSet{TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root, Operations: []objectmodel.Operation{
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, PathHint: tc.schemaPath, Value: map[string]any{"entity": "Job", "aspect": "runtime", "origin": "https://runtime.example", "fields": map[string]any{"status": map[string]any{"type": "string"}}}},
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/scoped", AspectName: "definition"}, PathHint: "public/definition.yaml", Value: map[string]any{"owner": "agent"}},
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/scoped", AspectName: "audit"}, PathHint: tc.siblingPath, Value: map[string]any{"rule": "fixed"}},
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
				return rd.Require(id, kernel.ErrKnowledgeRefUnresolved)
			}, reader.KnowledgeSetPin{SetID: "scoped", Repositories: map[kernel.RepositoryID]kernel.CommitID{raw.ID(): at}, Items: []reader.DatasetItem{{Repository: raw.ID(), Commit: at, Kind: "prefix", Prefix: "public"}}})
			lookup := &stateLookup{result: liveaccess.StateObservation{Value: map[string]any{"status": "running"}, Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "rev-1", ObservedAt: "2026-10-13T00:00:00Z"}}}
			values, err := readLogicalReference(t, liveservice.Open(scope, lookup, kernel.IdentityContext{Principal: "agent"}), objectmodel.Reference{Repository: raw.ID(), Object: "job/scoped", Locator: "/runtime/status"})
			if tc.wantErr {
				testkit.ExpectCode(t, err, kernel.ErrKnowledgeRefUnresolved)
				if len(lookup.requests) != 0 {
					t.Fatalf("unpublished evidence invoked runtime: %#v", lookup.requests)
				}
			} else if err != nil || len(values) != 1 || values[0].Value != "running" || len(lookup.requests) != 1 || lookup.requests[0].Binding.DeclarationCommit != at {
				t.Fatalf("permitted virtual Bound evidence: %#v %#v %v", values, lookup.requests, err)
			}
			if guard.reads["private/schema.yaml"] != 0 || guard.reads["private/sibling.yaml"] != 0 {
				t.Fatalf("auxiliary read escaped Dataset scope: %#v", guard.reads)
			}
		})
	}
}
