package service_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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

type partialReferenceStore struct {
	snapshot.Store
	snapshot.TreeStore
	reads     map[string][]kernel.CommitID
	afterRead func(string)
}

func (s *partialReferenceStore) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	s.reads[path] = append(s.reads[path], at)
	if strings.HasPrefix(path, "private/") {
		return nil, fmt.Errorf("logical root read an unpublished sibling: %s", path)
	}
	data, err := s.TreeStore.ReadFile(path, at)
	if s.afterRead != nil {
		s.afterRead(path)
	}
	return data, err
}

func (*partialReferenceStore) ListFiles(kernel.CommitID) ([]string, error) {
	return nil, fmt.Errorf("logical root must use bounded unit metadata")
}

func newPartialReferenceFixture(t *testing.T, dynamic bool) (*reader.Serving, *partialReferenceStore, kernel.CommitID) {
	t.Helper()
	raw := testkit.MakeTreeStore(t, "kr://logical/partial-root")
	writes := snapshot.NewRegistry()
	if err := writes.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(writes, nil)
	if err != nil {
		t.Fatal(err)
	}
	definition := objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/partial", AspectName: "definition"}, PathHint: "public/definition.yaml", Value: map[string]any{"owner": "fixed"}}
	operations := []objectmodel.Operation{
		definition,
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "job/partial", AspectName: "labels", MemberKey: "team"}, PathHint: "public/member.yaml", Value: map[string]any{"name": "data"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/partial", AspectName: "audit"}, PathHint: "private/audit.yaml", Value: map[string]any{"secret": "unpublished"}},
	}
	if dynamic {
		operations = append(operations,
			objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, PathHint: "public/schema.yaml", Value: map[string]any{"entity": "Job", "aspect": "runtime", "fields": map[string]any{"status": map[string]any{"type": "string"}}}},
			bindingfixture.WithPath(bindingfixture.PutDescriptor("resource/runtime", bindingfixture.Descriptor("https://runtime.example")), "public/descriptor.yaml"),
			bindingfixture.WithPath(bindingfixture.PutAspect("job/partial", "runtime", "schema/job.runtime", "resource/runtime"), "public/runtime.yaml"))
	}
	base := testkit.MustHead(t, raw, snapshot.DefaultRef)
	receipt, err := w.Commit("partial-root", objectmodel.ChangeSet{TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	commit := receipt.Result.NewCommit
	definition.Value = map[string]any{"owner": "changed after publication"}
	if _, err := w.Commit("advance-partial-root", objectmodel.ChangeSet{TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: commit, ExpectedTargetCommit: commit, Operations: []objectmodel.Operation{definition}}); err != nil {
		t.Fatal(err)
	}
	store := &partialReferenceStore{Store: raw, TreeStore: raw.(snapshot.TreeStore), reads: map[string][]kernel.CommitID{}}
	reads := snapshot.NewRegistry()
	if err := reads.Add(store); err != nil {
		t.Fatal(err)
	}
	rd := reader.NewReader(reads)
	serving := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return rd.Require(id, kernel.ErrObjectRefUnresolved)
	}, reader.DatasetPin{SetID: "partial-root", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{raw.ID(): commit}, Items: []reader.DatasetItem{{Repository: raw.ID(), Commit: commit, Kind: "prefix", Prefix: "public"}}})
	return serving, store, commit
}

func TestLogicalRootReferenceAssemblesOnlyPublishedUnitsAtTheFixedRevision(t *testing.T) {
	for _, test := range []struct {
		name    string
		dynamic bool
		failure bool
	}{
		{"static aspects and members", false, false},
		{"current external observation", true, false},
		{"external failure stays at its unit", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			serving, store, commit := newPartialReferenceFixture(t, test.dynamic)
			lookup := &stateLookup{result: liveaccess.StateObservation{Value: map[string]any{"status": "running"}, Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "runtime-7", ObservedAt: "2026-10-16T00:00:00Z"}}}
			if test.failure {
				lookup.err = kernel.Fail(kernel.ErrTemporaryUnavailable, "runtime unavailable")
			}
			ref := objectmodel.Reference{Repository: store.ID(), Object: "job/partial"}
			service := liveservice.Open(serving, lookup, kernel.IdentityContext{Principal: "consumer"})
			values, err := service.ReadReference(context.Background(), ref)
			if err != nil || len(values) != 1 {
				t.Fatalf("partial logical root is unreadable: values=%#v error=%v", values, err)
			}
			result := values[0]
			body := result.Value.(map[string]any)
			if !reflect.DeepEqual(body["definition"], map[string]any{"owner": "fixed"}) || !reflect.DeepEqual(body["labels"], map[string]any{"team": map[string]any{"name": "data"}}) || body["audit"] != nil {
				t.Fatalf("logical root followed HEAD or expanded its publication: %#v", result)
			}
			if result.Reference != "urn:kc:object:job/partial" || result.Locator != "" || result.Repository != store.ID() || result.Commit != commit || result.ObjectID != ref.Object || result.ObjectRef.Repository != store.ID() || result.ObjectRef.Object != ref.Object || result.Address != (objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: ref.Object}) || result.Digest != "" {
				t.Fatalf("selected root lost its logical position or invented a maintenance digest: %#v", result)
			}
			wantUnits := 2
			if test.dynamic {
				wantUnits++
			}
			if len(result.Declarations) != wantUnits || len(result.Units) != wantUnits {
				t.Fatalf("root lost selected maintenance coordinates: %#v", result)
			}
			for _, declaration := range result.Declarations {
				if declaration.Digest == "" || declaration.DeclarationDigest == "" || declaration.Address.AspectName == "audit" {
					t.Fatalf("root replaced the selected declaration: %#v", declaration)
				}
			}
			assertPartialRootRuntime(t, result, lookup, commit, test.dynamic, test.failure)
			ref.Locator = "/definition/owner"
			selected, err := service.ReadReference(context.Background(), ref)
			if err != nil || len(selected) != 1 || selected[0].Value != "fixed" || selected[0].Digest != kernel.CanonicalDigest(map[string]any{"owner": "fixed"}) {
				t.Fatalf("static leaf lost its complete maintenance unit digest: %#v %v", selected, err)
			}
			ref.Locator = "/audit"
			_, err = service.ReadReference(context.Background(), ref)
			testkit.ExpectCode(t, err, kernel.ErrObjectRefUnresolved)
			for path, commits := range store.reads {
				if strings.HasPrefix(path, "private/") {
					t.Fatalf("unselected sibling was read: %#v", store.reads)
				}
				for _, actual := range commits {
					if actual != commit {
						t.Fatalf("read %s followed HEAD: got %s want %s", path, actual, commit)
					}
				}
			}
		})
	}
}

func assertPartialRootRuntime(t *testing.T, result objectmodel.ObservedValue, lookup *stateLookup, commit kernel.CommitID, dynamic, failure bool) {
	t.Helper()
	if !dynamic {
		if len(result.Observations) != 0 || len(result.Failures) != 0 || len(lookup.requests) != 0 {
			t.Fatalf("static root consulted an external source: %#v", result)
		}
		return
	}
	if len(lookup.requests) != 1 || lookup.requests[0].Binding.DeclarationCommit != commit || lookup.requests[0].Binding.Repository != result.Repository {
		t.Fatalf("external unit lost its fixed declaration coordinates: %#v", lookup.requests)
	}
	value := result.Value.(map[string]any)["runtime"]
	if failure {
		failed, ok := value.(liveservice.ValueFailure)
		if !ok || failed.Error.Code != kernel.ErrTemporaryUnavailable || len(result.Failures) != 1 || result.Failures[0].Address.AspectName != "runtime" || len(result.Observations) != 0 {
			t.Fatalf("root swallowed a local unit failure: %#v", result)
		}
		return
	}
	if !reflect.DeepEqual(value, map[string]any{"status": "running"}) || len(result.Observations) != 1 || len(result.Failures) != 0 {
		t.Fatalf("root lost the current external value: %#v", result)
	}
	observation := result.Observations[0]
	if observation.DeclarationCommit != commit || observation.Basis.SourceRevision != "runtime-7" || observation.DeclarationDigest == "" || observation.DescriptorDigest == "" {
		t.Fatalf("root lost the external observation basis: %#v", observation)
	}
	for _, declaration := range result.Declarations {
		if declaration.Address == observation.Address && declaration.DeclarationDigest != observation.DeclarationDigest {
			t.Fatalf("root replaced the adopted binding declaration: %#v", result)
		}
	}
}

func TestPartialLogicalRootCancellationStopsBeforeTheNextUnitBody(t *testing.T) {
	serving, store, _ := newPartialReferenceFixture(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.afterRead = func(path string) {
		if path == "public/definition.yaml" {
			cancel()
		}
	}
	lookup := &stateLookup{}
	values, err := liveservice.Open(serving, lookup, kernel.IdentityContext{Principal: "consumer"}).ReadReference(ctx, objectmodel.Reference{Repository: store.ID(), Object: "job/partial"})
	if !errors.Is(err, context.Canceled) || len(values) != 0 || len(store.reads["public/definition.yaml"]) != 1 || len(store.reads["public/member.yaml"]) != 0 || len(store.reads["public/runtime.yaml"]) != 0 || len(lookup.requests) != 0 {
		t.Fatalf("cancelled root continued into another selected unit: values=%#v reads=%#v calls=%#v error=%v", values, store.reads, lookup.requests, err)
	}
}
