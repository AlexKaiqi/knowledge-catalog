package projection

import (
	"context"
	"strings"
	"testing"

	"kc/declarativeindex/internal/testfixture"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

// A failed reread of a condition unit is neither a match nor a business miss
// (discovery#U1): the candidate leaves the page, and the result says partial
// and names the unit instead of reporting a complete empty answer.
func TestDatasetStateRecheckFailureIsNamedNotSilentlyDropped(t *testing.T) {
	setup := testkit.NewSetup(t, "")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/orders", AspectName: "runtime"}
	descriptor := bindingfixture.Descriptor("https://scheduler.example")
	descriptor["identity"] = string(objectmodel.IdentityShared)
	commit, err := setup.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: setup.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: setup.RootCommitID, ExpectedTargetCommit: setup.RootCommitID, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.definition"}, Value: map[string]any{"entity": "Job", "aspect": "definition", "fields": map[string]any{"owner": map[string]any{"type": "string"}}}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, Value: map[string]any{"entity": "Job", "aspect": "runtime", "fields": map[string]any{"status": map[string]any{"type": "string"}}}},
		testkit.IndexField("schema/job.definition", "owner", "keyword"),
		testkit.IndexField("schema/job.runtime", "status", "keyword"),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: address.ObjectID, AspectName: "definition"}, SchemaRef: "schema/job.definition", Value: map[string]any{"owner": "data"}},
		bindingfixture.PutDescriptor("resource/scheduler", descriptor),
		bindingfixture.Bind(address, "schema/job.runtime", "resource/scheduler"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	repo := setup.Repo
	engines := map[string]*datasetStateEngine{}
	idx := NewIndexEngine(t.TempDir(), func(name string, _ kernel.RepositoryID) (ports.Engine, error) {
		if engines[name] == nil {
			engines[name] = &datasetStateEngine{StateEngine: &testfixture.StateEngine{Docs: map[objectmodel.ObjectID]ports.CompiledDoc{}}}
		}
		return engines[name], nil
	})
	t.Cleanup(func() { _ = idx.Close() })
	lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo.Repository, nil }
	pin := reader.DatasetPin{SetID: "recheck", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit}}
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
		t.Fatal(err)
	}
	state := &testfixture.StateLookup{
		Value: map[string]any{"status": "running"},
		Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-10-20T00:00:00Z"},
	}
	if _, err := idx.RefreshDatasetState(context.Background(), lookup, pin, sourcebridge.Source{Lookup: state}); err != nil {
		t.Fatal(err)
	}
	request := retrieval.SearchOf(retrieval.SearchEQ("status", "running"))
	recheck := func(failures ...objectmodel.UnitReadFailure) context.Context {
		return ports.WithCandidateRecheck(context.Background(), func(_ context.Context, hit retrieval.ObjectHit) (retrieval.ObjectHit, []objectmodel.UnitReadFailure, error) {
			return hit, failures, nil
		})
	}

	failed, err := idx.SearchDataset(recheck(objectmodel.UnitReadFailure{Address: address, Code: kernel.ErrTemporaryUnavailable, Message: "scheduler down"}), lookup, pin, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed.Hits) != 0 || failed.Completeness != retrieval.CompletenessPartial {
		t.Fatalf("failed condition reread must leave the page as partial: %#v", failed)
	}
	claims := strings.Join(failed.Claims, "\n")
	if !strings.Contains(claims, "job/orders#/runtime") || !strings.Contains(claims, string(kernel.ErrTemporaryUnavailable)) || strings.Contains(claims, "\u001f") {
		t.Fatalf("failed unit is not named readably: %q", failed.Claims)
	}

	other := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: address.ObjectID, AspectName: "definition"}
	unrelated, err := idx.SearchDataset(recheck(objectmodel.UnitReadFailure{Address: other, Code: kernel.ErrTemporaryUnavailable, Message: "elsewhere"}), lookup, pin, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(unrelated.Hits) != 1 || unrelated.Completeness != retrieval.CompletenessComplete {
		t.Fatalf("failure outside the condition aspects changed the answer: %#v", unrelated)
	}
	if len(unrelated.Claims) != 1 || !strings.Contains(unrelated.Claims[0], "job/orders#/definition") {
		t.Fatalf("failure outside the conditions was not named beside the kept hit: %q", unrelated.Claims)
	}
	if len(unrelated.ExternalConditions) != 1 || unrelated.ExternalConditions[0].Field.Path != "status" || unrelated.ExternalConditions[0].OldestObservedAt == "" {
		t.Fatalf("the condition answered from external values must be named with its observation age: %#v", unrelated.ExternalConditions)
	}
}

// datasetStateEngine reports each candidate at its source coordinates, as a
// Dataset physical engine does; the shared fixture omits them.
type datasetStateEngine struct{ *testfixture.StateEngine }

func (e *datasetStateEngine) Retrieve(req ports.RetrieveRequest) (ports.CandidatePage, error) {
	page, err := e.StateEngine.Retrieve(req)
	for i, candidate := range page.Candidates {
		doc := e.Docs[candidate.ObjectID]
		page.Candidates[i].Repository, page.Candidates[i].Basis = doc.SourceRepository, doc.SourceCommit
		if doc.SourceRepository == "" {
			page.Candidates[i].Repository = req.Spec.Repository
		}
	}
	return page, err
}
