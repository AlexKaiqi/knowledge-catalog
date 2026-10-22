package plugin_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	"kc/snapshot"
)

// Goals: index-control U3/U4, declarative-index U3 and plugin-extension's
// effective demand contract. HEAD failure cannot gate an accepted fixed basis;
// stale durable work cannot grant demand; one basis must not block another;
// State recovery must use the fixed declaration and independent worker identity.
// Non-Goals: no query-triggered rebuild, source publication, or fake registrar.
func TestIndexControlRestoresAcceptedDatasetBasisWhenCurrentHeadReadFails(t *testing.T) {
	f := newControlRecoveryFixture(t)
	source := f.base.setups[0]
	at := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	headFailure := kernel.Fail(kernel.ErrTemporaryUnavailable, "current HEAD authority unavailable")
	blocked := false
	f.lookup = func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		repo, err := f.base.lookup(id)
		if err == nil && blocked && id == source.RepositoryID {
			repo = failingControlHead(repo, headFailure)
		}
		return repo, err
	}
	runtime := f.open(t, source.RepositoryID)
	f.publish(t, runtime, "fixed-a", source.RepositoryID, at)
	f.advance(t, 0, "b")
	evidence := f.accepted(t)
	f.discardFixed(t, source.RepositoryID, at)
	blocked = true
	controlRepo, err := f.lookup(source.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlRepo.Head(snapshot.DefaultRef); !errors.Is(err, headFailure) {
		t.Fatalf("fault did not isolate the real current HEAD read: %v", err)
	}
	if schema, err := controlRepo.Read("schema/article", at); err != nil || schema.Commit != at {
		t.Fatalf("HEAD fault also blocked the fixed Schema: %#v %v", schema, err)
	}
	if body, err := controlRepo.Read("article/one", at); err != nil || body.Value.(map[string]any)["body"] != "runbook source 1 at a" {
		t.Fatalf("HEAD fault also blocked the fixed canonical body: %#v %v", body, err)
	}
	if err := runtime.plugins.CatchUp(context.Background()); !errors.Is(err, headFailure) {
		t.Errorf("fixed recovery replaced the independent HEAD fault: %v", err)
	}
	f.assertSnapshot(t, runtime, source.RepositoryID, at, "runbook source 1 at a")
	f.assertAccepted(t, evidence)
}

func TestIndexControlReopeningEmptyDemandDoesNotExecuteRetiredWork(t *testing.T) {
	for _, lane := range []string{"snapshot-desire", "state-notice"} {
		t.Run(lane, func(t *testing.T) {
			f := newControlRecoveryFixture(t)
			a, b := f.base.setups[0], f.base.setups[1]
			at := testkit.MustHead(t, a.Repo, snapshot.DefaultRef)
			var address objectmodel.Address
			lookup := &controlStateLookup{status: "old-observation", revision: "r1"}
			if lane == "state-notice" {
				at, address = controlBoundJob(t, f)
			}
			runtime := f.open(t, a.RepositoryID, b.RepositoryID)
			f.publish(t, runtime, "retired-a", a.RepositoryID, at)
			if lane == "snapshot-desire" {
				if err := runtime.plugins.Desire(a.RepositoryID, at); err != nil {
					t.Fatal(err)
				}
				targets, err := runtime.plugins.Targets()
				if err != nil || len(targets) != 1 || targets[0].Status != index.TargetPending || targets[0].DesiredCommit != at {
					t.Fatalf("actual durable Desire was not pending: %#v %v", targets, err)
				}
			} else {
				runtime.plugins.SetObservationSource(controlObservationSource(lookup))
				if err := runtime.plugins.CatchUp(context.Background()); err != nil {
					t.Fatal(err)
				}
				assertControlState(t, f, runtime, lookup, at, "old-observation", "a-owner", "r1")
				if err := runtime.plugins.Notify(index.ChangeNotice{Repository: a.RepositoryID, Address: &address, SourceRevision: "r2"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.base.cat.RetireDataset("retired-a"); err != nil {
				t.Fatal(err)
			}
			if err := runtime.plugins.Close(); err != nil {
				t.Fatal(err)
			}
			reopened := f.open(t, a.RepositoryID, b.RepositoryID)
			// An explicit empty partition is an authority decision. Reopening
			// the same durable ledger must not use its old targets as fallback.
			reopened.controller.SetInventory(func() ([]kernel.RepositoryID, error) { return []kernel.RepositoryID{}, nil })
			reopened.plugins.SetObservationSource(controlObservationSource(lookup))
			f.mu.Lock()
			beforeBuilds := len(f.builds)
			f.mu.Unlock()
			beforeCalls := len(lookup.requests)
			evidence := f.accepted(t)
			if err := reopened.plugins.CatchUp(context.Background()); err != nil {
				t.Errorf("empty effective demand failed instead of ignoring retired work: %v", err)
			}
			f.mu.Lock()
			staleBuilds := append([]controlPhysicalBuild(nil), f.builds[beforeBuilds:]...)
			f.mu.Unlock()
			if len(staleBuilds) != 0 || len(lookup.requests) != beforeCalls {
				t.Errorf("empty effective demand executed retired work: physical builds=%#v State calls before=%d after=%d", staleBuilds, beforeCalls, len(lookup.requests))
			}
			f.assertAccepted(t, evidence)
			// The same actual Controller must still maintain a positive demand;
			// disabling all work would also satisfy the negative assertion.
			reopened.controller.SetInventory(func() ([]kernel.RepositoryID, error) { return []kernel.RepositoryID{b.RepositoryID}, nil })
			atB := testkit.MustHead(t, b.Repo, snapshot.DefaultRef)
			if err := reopened.plugins.CatchUp(context.Background()); err != nil {
				t.Fatal(err)
			}
			f.assertSnapshot(t, reopened, b.RepositoryID, atB, "runbook source 2 at a")
			f.assertAccepted(t, evidence)
		})
	}
}

func TestIndexControlFailureOfOneFixedBasisDoesNotBlockAnother(t *testing.T) {
	f := newControlRecoveryFixture(t)
	source := f.base.setups[0]
	a := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	runtime := f.open(t, source.RepositoryID)
	f.publish(t, runtime, "fixed-a", source.RepositoryID, a)
	b := f.advance(t, 0, "b")
	f.publish(t, runtime, "fixed-b", source.RepositoryID, b)
	f.advance(t, 0, "c")
	evidence := f.accepted(t)
	f.discardFixed(t, source.RepositoryID, a)
	f.discardFixed(t, source.RepositoryID, b)
	fault := kernel.Fail(kernel.ErrTemporaryUnavailable, "first fixed basis physical publication failed")
	var failed kernel.CommitID
	f.mu.Lock()
	f.buildFail = func(build controlPhysicalBuild) error {
		if (build.Meta.Basis == a || build.Meta.Basis == b) && strings.HasSuffix(string(build.Repository), "@"+string(build.Meta.Basis)) {
			if failed == "" {
				failed = build.Meta.Basis
			}
			if build.Meta.Basis == failed {
				return fault
			}
		}
		return nil
	}
	f.mu.Unlock()
	catchUpErr := runtime.plugins.CatchUp(context.Background())
	if failed == "" {
		t.Fatal("actual Dataset recovery did not attempt either lost fixed projection")
	}
	other, body := b, "runbook source at b"
	if failed == b {
		other, body = a, "runbook source 1 at a"
	}
	f.assertSnapshot(t, runtime, source.RepositoryID, other, body)
	if !errors.Is(catchUpErr, fault) {
		t.Errorf("recovery lost the actual fixed-basis failure: returned=%v", catchUpErr)
	}
	repo, err := f.base.lookup(source.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.plugins.CheckSearchProjectionAt(repo, failed); err == nil {
		t.Error("failed physical publication was falsely marked READY")
	}
	f.assertAccepted(t, evidence)
	f.mu.Lock()
	f.buildFail = nil
	f.mu.Unlock()
	if err := runtime.plugins.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.assertSnapshot(t, runtime, source.RepositoryID, a, "runbook source 1 at a")
	f.assertSnapshot(t, runtime, source.RepositoryID, b, "runbook source at b")
	f.assertAccepted(t, evidence)
}

func TestIndexControlColdStateRecoveryUsesAcceptedDeclarationAfterHeadChanges(t *testing.T) {
	f := newControlRecoveryFixture(t)
	source := f.base.setups[0]
	a, _ := controlBoundJob(t, f)
	runtime := f.open(t, source.RepositoryID)
	f.publish(t, runtime, "fixed-state-a", source.RepositoryID, a)
	lookup := &controlStateLookup{status: "old-observation", revision: "r1"}
	runtime.plugins.SetObservationSource(controlObservationSource(lookup))
	if err := runtime.plugins.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertControlState(t, f, runtime, lookup, a, "old-observation", "a-owner", "r1")
	if files := f.stateFiles(t); files == 0 {
		t.Fatal("actual State maintenance did not create Serving State media")
	}
	b, err := source.Writer.Commit("remove-head-state-binding", objectmodel.CommitChangeSet{
		TargetRepository: source.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: a, ExpectedTargetCommit: a,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/control", AspectName: "runtime"}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/control", AspectName: "definition"}, SchemaRef: "schema/job.definition", Value: map[string]any{"owner": "b-owner"}},
		},
	})
	if err != nil || b.Result.NewCommit == a {
		t.Fatalf("fixture did not publish different HEAD declarations: %#v %v", b, err)
	}
	if err := runtime.plugins.Close(); err != nil {
		t.Fatal(err)
	}
	if files := f.stateFiles(t); files == 0 {
		t.Fatal("Close removed durable accepted observation values")
	}
	evidence := f.accepted(t)
	lookup.status, lookup.revision = "new-observation", "r2"
	beforeCalls := len(lookup.requests)
	reopened := f.open(t, source.RepositoryID)
	reopened.plugins.SetObservationSource(controlObservationSource(lookup))
	if _, ready := reopened.plugins.StateView(source.RepositoryID, a); ready {
		t.Fatal("reopened Index claimed process-local Serving State survived Close")
	}
	if err := reopened.plugins.CatchUp(context.Background()); err != nil {
		t.Errorf("cold maintenance failed: %v", err)
	}
	if len(lookup.requests) != beforeCalls {
		t.Errorf("cold recovery re-observed an accepted fixed observation: before=%d after=%d", beforeCalls, len(lookup.requests))
	}
	assertControlState(t, f, reopened, lookup, a, "old-observation", "a-owner", "r1")
	f.assertAccepted(t, evidence)
}

func controlBoundJob(t *testing.T, f *controlRecoveryFixture) (kernel.CommitID, objectmodel.Address) {
	t.Helper()
	source := f.base.setups[0]
	base := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/control", AspectName: "runtime"}
	receipt, err := source.Writer.Commit("control-state-declaration-a", objectmodel.CommitChangeSet{
		TargetRepository: source.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.definition"}, Value: map[string]any{"entity": "Job", "aspect": "definition", "fields": map[string]any{"owner": map[string]any{"type": "string"}}}},
			testkit.IndexField("schema/job.definition", "owner", "keyword"),
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, Value: map[string]any{"entity": "Job", "aspect": "runtime", "fields": map[string]any{"status": map[string]any{"type": "string"}}}},
			testkit.IndexField("schema/job.runtime", "status", "keyword"),
			bindingfixture.PutDescriptor("resource/state", bindingfixture.Descriptor("https://old-state.example")),
			bindingfixture.PutAspect("job/control", "runtime", "schema/job.runtime", "resource/state"),
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/control", AspectName: "definition"}, SchemaRef: "schema/job.definition", Value: map[string]any{"owner": "a-owner"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt.Result.NewCommit, address
}

func assertControlState(t *testing.T, f *controlRecoveryFixture, runtime *controlRecoveryRuntime, lookup *controlStateLookup, at kernel.CommitID, status, owner, sourceRevision string) {
	t.Helper()
	id := f.base.setups[0].RepositoryID
	repo, err := f.base.lookup(id)
	if err != nil {
		t.Fatal(err)
	}
	revision, ready := runtime.plugins.StateView(id, at)
	if !ready || revision == "" {
		t.Errorf("fixed State projection was not actually restored: source=%s basis=%s", id, at)
		return
	}
	query := retrieval.SearchOf(
		retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/job.runtime", Aspect: "runtime", Path: "status"}, Value: status},
		retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/job.definition", Aspect: "definition", Path: "owner"}, Value: owner},
	)
	beforeCalls := len(lookup.requests)
	result, err := runtime.plugins.SearchStateAtRevisionContext(context.Background(), repo, at, revision, query)
	if len(lookup.requests) != beforeCalls {
		t.Errorf("State query performed external observation: before=%d after=%d", beforeCalls, len(lookup.requests))
	}
	if err != nil || len(result.Hits) != 1 || result.Completeness != retrieval.CompletenessComplete || result.SearchView.Snapshots[id] != at {
		t.Errorf("fixed State search did not return complete declared body: %#v %v", result, err)
		return
	}
	hit := result.Hits[0]
	canonical, err := repo.Read("job/control", at)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]any{}
	for aspect, value := range canonical.Value.(map[string]any) {
		expected[aspect] = value
	}
	expected["runtime"] = map[string]any{"status": status}
	if hit.Object.Repository != id || hit.Object.Commit != at || hit.Object.Address != canonical.Address || hit.Object.ObjectRef != canonical.ObjectRef || !reflect.DeepEqual(hit.Object.Value, expected) || hit.Version.DeclarationCommit != at || len(hit.Version.Observations) != 1 || hit.Version.Observations[0].Basis.SourceRevision != sourceRevision {
		t.Errorf("fixed State result mixed canonical body or observation: %#v", hit)
	}
}

var _ liveaccess.StateLookup = (*controlStateLookup)(nil)
