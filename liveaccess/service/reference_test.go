package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
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

type logicalReferenceReader interface {
	ReadReference(context.Context, objectmodel.Reference) ([]objectmodel.ObservedValue, error)
}

func readLogicalReference(t *testing.T, service *liveservice.Service, ref objectmodel.Reference) ([]objectmodel.ObservedValue, error) {
	t.Helper()
	port, ok := any(service).(logicalReferenceReader)
	if !ok {
		t.Fatal("logical READ has no Reference port; canonical selection loses State/Stream semantics")
	}
	return port.ReadReference(context.Background(), ref)
}

type referenceRuntimeRequest struct {
	Binding struct {
		Repository kernel.RepositoryID `json:"repository"`
		Commit     kernel.CommitID     `json:"declarationCommit"`
		Address    objectmodel.Address `json:"address"`
	} `json:"binding"`
}

func newLogicalReferenceFixture(t *testing.T, origin string) (*reader.Serving, []kernel.RepositoryID, map[kernel.RepositoryID]kernel.CommitID) {
	t.Helper()
	registry := snapshot.NewRegistry()
	ids := []kernel.RepositoryID{"kr://logical/source-a", "kr://logical/source-b"}
	commits := map[kernel.RepositoryID]kernel.CommitID{}
	for _, id := range ids {
		raw := testkit.MakeTreeStore(t, string(id))
		if err := registry.Add(raw); err != nil {
			t.Fatal(err)
		}
		w, err := writer.NewWriter(registry, nil)
		if err != nil {
			t.Fatal(err)
		}
		root := testkit.MustHead(t, raw, snapshot.DefaultRef)
		ops := []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/shared", AspectName: "definition"}, Value: map[string]any{"owner": string(id)}}}
		for _, aspect := range []string{"runtime", "audit"} {
			ops = append(ops, objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID("schema/job." + aspect)}, Value: map[string]any{
				"entity": "Job", "aspect": aspect, "origin": origin,
				"fields": map[string]any{"status": map[string]any{"type": "string"}},
			}})
		}
		receipt, err := w.Commit("logical-reference-seed", objectmodel.ChangeSet{TargetRepository: id, TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root, Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		commits[id] = receipt.Result.NewCommit
	}
	rd := reader.NewReader(registry)
	return reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return rd.Require(id, kernel.ErrKnowledgeRefUnresolved)
	}, reader.KnowledgeSetPin{SetID: "logical-reference", Repositories: commits, Items: reader.WholeRepositoryItems(commits)}), ids, commits
}

func TestLogicalReferencePreservesStateAndSelectsOnlyExplicitSource(t *testing.T) {
	var mu sync.Mutex
	calls := []referenceRuntimeRequest{}
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req referenceRuntimeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			http.Error(w, "bad request", 400)
			return
		}
		if r.URL.Path != "/v1/access" || r.Method != http.MethodPost {
			t.Errorf("runtime request %s %s", r.Method, r.URL.Path)
		}
		mu.Lock()
		calls = append(calls, req)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"status": string(req.Binding.Repository) + ":" + req.Binding.Address.AspectName}, "basis": objectmodel.ObservationBasis{BindingGeneration: "generation-1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "source-rev-7", ObservedAt: "2026-10-13T00:00:00Z"}})
	}))
	defer runtime.Close()
	base, ids, commits := newLogicalReferenceFixture(t, runtime.URL)
	svc := liveservice.Open(base, liveaccess.NewHTTPStateLookup(runtime.Client()), kernel.IdentityContext{Principal: "agent"})
	old, err := svc.Read(context.Background(), "job/shared", nil)
	if err != nil || len(old) != 2 {
		t.Fatalf("existing logical READ: %#v %v", old, err)
	}
	mu.Lock()
	if len(calls) != 4 {
		t.Errorf("old READ runtime calls %#v", calls)
	}
	calls = nil
	mu.Unlock()
	ref := objectmodel.Reference{Repository: ids[0], Object: "job/shared"}
	root, err := readLogicalReference(t, svc, ref)
	if err != nil || len(root) != 1 {
		t.Fatalf("Reference root: %#v %v", root, err)
	}
	var oldA objectmodel.ObservedValue
	for _, v := range old {
		if v.Repository == ids[0] {
			oldA = v
		}
	}
	if !reflect.DeepEqual(root[0].Value, oldA.Value) || root[0].Repository != ids[0] || root[0].Commit != commits[ids[0]] || len(root[0].Observations) != 2 {
		t.Fatalf("root lost existing logical/source semantics: old=%#v new=%#v", oldA, root[0])
	}
	if root[0].Digest != "" {
		t.Fatalf("dynamic assembled value claims static maintenance digest: %#v", root[0])
	}
	mu.Lock()
	if len(calls) != 2 {
		t.Errorf("explicit root calls %#v", calls)
	}
	for _, c := range calls {
		if c.Binding.Repository != ids[0] || c.Binding.Commit != commits[ids[0]] {
			t.Errorf("wrong source or pin: %#v", c)
		}
	}
	calls = nil
	mu.Unlock()
	ref.Locator = "/runtime/status"
	deep, err := readLogicalReference(t, svc, ref)
	if err != nil || len(deep) != 1 {
		t.Fatalf("virtual Bound leaf: %#v %v", deep, err)
	}
	wantAddr := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/shared", AspectName: "runtime"}
	if deep[0].Value != string(ids[0])+":runtime" || deep[0].Address != wantAddr || deep[0].Digest != "" || deep[0].Locator != ref.Locator || len(deep[0].Observations) != 1 || deep[0].Observations[0].Basis.SourceRevision != "source-rev-7" {
		t.Fatalf("deep Bound semantics %#v", deep[0])
	}
	mu.Lock()
	if len(calls) != 1 || calls[0].Binding.Repository != ids[0] || calls[0].Binding.Address != wantAddr {
		t.Errorf("selected leaf hydrated siblings or guessed source: %#v", calls)
	}
	calls = nil
	mu.Unlock()
	ref.Locator = "/definition/owner"
	static, err := readLogicalReference(t, svc, ref)
	if err != nil || len(static) != 1 || static[0].Value != string(ids[0]) || static[0].Digest != kernel.CanonicalDigest(map[string]any{"owner": string(ids[0])}) || len(static[0].Observations) != 0 {
		t.Fatalf("static selected maintenance basis: %#v %v", static, err)
	}
	mu.Lock()
	if len(calls) != 0 {
		t.Errorf("static leaf called runtime: %#v", calls)
	}
	mu.Unlock()
}

func TestLogicalReferenceRequiresRuntimeAndDoesNotInventBoundFacets(t *testing.T) {
	base, ids, _ := newLogicalReferenceFixture(t, "https://runtime.example")
	svc := liveservice.Open(base, nil, kernel.IdentityContext{Principal: "agent"})
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/shared", AspectName: "runtime"}
	values, err := svc.ReadAddress(context.Background(), address)
	if err != nil || len(values) != len(ids) {
		t.Fatalf("missing runtimes erased one source's local result: %#v %v", values, err)
	}
	for _, value := range values {
		assertFailedUnitRead(t, []objectmodel.ObservedValue{value}, nil, address, kernel.ErrCapabilityUnsatisfied)
	}
	for _, locator := range []string{"", "/runtime/status"} {
		values, err = readLogicalReference(t, svc, objectmodel.Reference{Repository: ids[0], Object: "job/shared", Locator: locator})
		assertFailedUnitRead(t, values, err, address, kernel.ErrCapabilityUnsatisfied)
	}
	lookup := &stateLookup{}
	withRuntime := liveservice.Open(base, lookup, kernel.IdentityContext{Principal: "agent"})
	_, err = readLogicalReference(t, withRuntime, objectmodel.Reference{Repository: ids[0], Object: "job/shared", Locator: "/unknown/status"})
	testkit.ExpectCode(t, err, kernel.ErrKnowledgeRefUnresolved)
	if len(lookup.requests) != 0 {
		t.Fatalf("unconfirmed facet contacted runtime: %#v", lookup.requests)
	}
	_, err = readLogicalReference(t, withRuntime, objectmodel.Reference{Repository: "kr://unpublished", Object: "job/shared", Locator: "/runtime/status"})
	testkit.ExpectCode(t, err, kernel.ErrKnowledgeRefUnresolved)
}

type streamReferenceRepo struct {
	objectmodel.Repository
	unit   objectmodel.Unit
	commit kernel.CommitID
}

func (r *streamReferenceRepo) Read(id objectmodel.ObjectID, at kernel.CommitID) (objectmodel.KnowledgeValue, error) {
	return objectmodel.AssembleObjectValue(r.ID(), id, at, []objectmodel.Unit{r.unit})
}
func (r *streamReferenceRepo) ReadReferenceUnits(ref objectmodel.Reference, at kernel.CommitID) ([]objectmodel.Unit, error) {
	return []objectmodel.Unit{r.unit}, nil
}

func TestLogicalReferenceRejectsStreamBeforeRuntime(t *testing.T) {
	base, id, commit, address := setupServing(t)
	repo, err := base.Member(id)
	if err != nil {
		t.Fatal(err)
	}
	unit := objectmodel.Unit{ObjectID: address.ObjectID, Address: address, Value: map[string]any{}, Digest: kernel.CanonicalDigest(map[string]any{}), ValueSource: &objectmodel.ValueSource{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{Mode: objectmodel.BindingStream, Runtime: "scheduler", Protocol: "scheduler/v1", Operations: map[string]objectmodel.BindingOperation{"read": {Call: "job.status"}}}}}
	native := &streamReferenceRepo{Repository: repo, unit: unit, commit: commit}
	commits := map[kernel.RepositoryID]kernel.CommitID{id: commit}
	scope := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return native, nil }, reader.KnowledgeSetPin{SetID: "stream", Repositories: commits, Items: reader.WholeRepositoryItems(commits)})
	lookup := &stateLookup{}
	svc := liveservice.Open(scope, lookup, kernel.IdentityContext{Principal: "agent"})
	values, err := svc.Read(context.Background(), address.ObjectID, nil)
	assertFailedUnitRead(t, values, err, address, kernel.ErrCapabilityUnsatisfied)
	for _, locator := range []string{"", "/runtime/status"} {
		values, err = readLogicalReference(t, svc, objectmodel.Reference{Repository: id, Object: address.ObjectID, Locator: locator})
		assertFailedUnitRead(t, values, err, address, kernel.ErrCapabilityUnsatisfied)
	}
	if len(lookup.requests) != 0 {
		t.Fatalf("Stream ordinary Reference contacted runtime: %#v", lookup.requests)
	}
}

type selectedStateReferenceRepo struct {
	objectmodel.Repository
	unit       objectmodel.Unit
	wholeReads int
}

func (r *selectedStateReferenceRepo) Read(id objectmodel.ObjectID, at kernel.CommitID) (objectmodel.KnowledgeValue, error) {
	if id == r.unit.ObjectID {
		r.wholeReads++
		return objectmodel.KnowledgeValue{}, fmt.Errorf("selected binding must not reread the whole object or its unselected siblings")
	}
	return r.Repository.Read(id, at)
}

func (r *selectedStateReferenceRepo) ReadReferenceUnits(objectmodel.Reference, kernel.CommitID) ([]objectmodel.Unit, error) {
	return []objectmodel.Unit{r.unit}, nil
}

func (r *selectedStateReferenceRepo) ReadAddress(address objectmodel.Address, at kernel.CommitID) (objectmodel.KnowledgeValue, error) {
	if address != r.unit.Address {
		return r.Repository.ReadAddress(address, at)
	}
	value, err := objectmodel.AssembleObjectValue(r.ID(), address.ObjectID, at, []objectmodel.Unit{r.unit})
	value.Address, value.Value, value.Digest = address, r.unit.Value, r.unit.Digest
	return value, err
}

func TestLogicalReferenceHydratesActualSelectedUnitBeforeMissingLeafWithoutRereadingObject(t *testing.T) {
	base, id, commit, address := setupServing(t)
	repo, err := base.Member(id)
	if err != nil {
		t.Fatal(err)
	}
	placeholder := map[string]any{"cached": "declaration only"}
	selectedRepo := &selectedStateReferenceRepo{Repository: repo, unit: objectmodel.Unit{ObjectID: address.ObjectID, Address: address, Value: placeholder, Digest: kernel.CanonicalDigest(placeholder), SchemaRef: "schema/job.runtime", ValueSource: &objectmodel.ValueSource{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{Mode: objectmodel.BindingState, Runtime: "scheduler", Protocol: "scheduler/v1", Operations: map[string]objectmodel.BindingOperation{"read": {Call: "job.status"}}}}}}
	commits := map[kernel.RepositoryID]kernel.CommitID{id: commit}
	scope := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return selectedRepo, nil }, reader.KnowledgeSetPin{SetID: "selected-actual", Repositories: commits, Items: reader.WholeRepositoryItems(commits)})
	lookup := &stateLookup{result: liveaccess.StateObservation{Value: map[string]any{"status": "observed-running"}, Basis: objectmodel.ObservationBasis{BindingGeneration: "runtime-7", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "runtime-rev-8", ObservedAt: "2026-10-13T00:00:00Z"}}}
	values, err := readLogicalReference(t, liveservice.Open(scope, lookup, kernel.IdentityContext{Principal: "agent"}), objectmodel.Reference{Repository: id, Object: address.ObjectID, Locator: "/runtime/status"})
	if err != nil || len(values) != 1 || values[0].Value != "observed-running" || values[0].Digest != "" || values[0].Address != address || len(values[0].Observations) != 1 || values[0].Observations[0].Basis.SourceRevision != "runtime-rev-8" {
		t.Fatalf("selected actual Binding unit: %#v %v", values, err)
	}
	if selectedRepo.wholeReads != 0 || len(lookup.requests) != 1 || lookup.requests[0].Binding.Repository != id || lookup.requests[0].Binding.DeclarationCommit != commit {
		t.Fatalf("selected unit reread object/source: whole=%d runtime=%#v", selectedRepo.wholeReads, lookup.requests)
	}
	raw, err := selectedRepo.ReadAddress(address, commit)
	if err != nil {
		t.Fatal(err)
	}
	result, err := liveservice.HydrateRepositoryRead(context.Background(), selectedRepo, raw, lookup, objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}}, nil)
	if err != nil || !reflect.DeepEqual(result.Value, lookup.result.Value) || result.Address != address || selectedRepo.wholeReads != 0 || len(lookup.requests) != 2 {
		t.Fatalf("Repository hydration did not adopt its already read Aspect declaration: %#v whole=%d runtime=%#v %v", result, selectedRepo.wholeReads, lookup.requests, err)
	}
}

func TestLogicalReferenceDoesNotExpandBindingExecutionToEntityOrSchema(t *testing.T) {
	base, id, commit, _ := setupServing(t)
	repo, err := base.Member(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, locator string
		address       objectmodel.Address
	}{
		{"entity", "/status", objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "job/no-entity-binding"}},
		{"schema", "/runtime/status", objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "schema/no-binding", AspectName: "runtime"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			placeholder := map[string]any{"cached": "fixed"}
			source := &objectmodel.ValueSource{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{Mode: objectmodel.BindingState, Runtime: "scheduler", Protocol: "scheduler/v1", Operations: map[string]objectmodel.BindingOperation{"read": {Call: "job.status"}}}}
			selectedRepo := &selectedStateReferenceRepo{Repository: repo, unit: objectmodel.Unit{ObjectID: tc.address.ObjectID, Address: tc.address, Value: placeholder, Digest: kernel.CanonicalDigest(placeholder), ValueSource: source}}
			commits := map[kernel.RepositoryID]kernel.CommitID{id: commit}
			scope := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return selectedRepo, nil }, reader.KnowledgeSetPin{SetID: "no-expanded-execution", Repositories: commits, Items: reader.WholeRepositoryItems(commits)})
			lookup := &stateLookup{result: liveaccess.StateObservation{Value: map[string]any{"status": "forbidden-execution"}, Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationLatestOnly, ObservedAt: "2026-10-13T00:00:00Z"}}}
			values, err := readLogicalReference(t, liveservice.Open(scope, lookup, kernel.IdentityContext{Principal: "agent"}), objectmodel.Reference{Repository: id, Object: tc.address.ObjectID, Locator: tc.locator})
			assertFailedUnitRead(t, values, err, tc.address, kernel.ErrUsageInvalid)
			if len(lookup.requests) != 0 {
				t.Fatalf("Reference expanded unsupported Binding target: %#v", lookup.requests)
			}
		})
	}
}
