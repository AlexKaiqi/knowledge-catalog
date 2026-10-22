package projection

import (
	"context"
	"errors"
	"os"
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

func observationQueryFixture(t *testing.T) (objectmodel.Repository, kernel.CommitID) {
	t.Helper()
	repo, at, _ := testfixture.StateProjection(t)
	writer := repo.(interface {
		ApplyObjectCommit(objectmodel.CommitChangeSet) (kernel.CommitID, error)
	})
	descriptor := bindingfixture.Descriptor("https://scheduler.example")
	descriptor["identity"] = string(objectmodel.IdentityShared)
	operations := []objectmodel.Operation{bindingfixture.PutDescriptor(testfixture.StateDescriptor, descriptor)}
	for _, id := range []objectmodel.ObjectID{"job/second", "job/third"} {
		operations = append(operations, objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "definition"}, SchemaRef: "schema/job.definition", Value: map[string]any{"owner": "data"}}, bindingfixture.Bind(objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "runtime"}, "schema/job.runtime", testfixture.StateDescriptor))
	}
	at, err := writer.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	return repo.(*testkit.ObjectRepository).Repository, at
}

func TestLiveObservationSearchContinuationKeepsOriginalGenerationAndValues(t *testing.T) {
	for _, scope := range []string{"repository", "dataset"} {
		for _, storage := range []string{"persistent", "anonymous"} {
			t.Run(scope+"/"+storage, func(t *testing.T) {
				repo, at := observationQueryFixture(t)
				directory := ""
				if storage == "persistent" {
					directory = t.TempDir()
				}
				opener := testfixture.LiveEngineOpener(t)
				idx := NewIndexEngine(directory, opener)
				defer func() { _ = idx.Close() }()
				lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }
				pin := reader.DatasetPin{SetID: "observation-search", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}}
				pin.Items = reader.WholeRepositoryItems(pin.Repositories)
				if scope == "dataset" {
					if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
						t.Fatal(err)
					}
				}
				observed := &testfixture.StateLookup{Value: map[string]any{"status": "running"}, Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-10-21T00:00:00Z"}}
				refresh := func() (ports.StateSync, error) {
					if scope == "dataset" {
						return idx.RefreshDatasetState(context.Background(), lookup, pin, sourcebridge.Source{Lookup: observed})
					}
					return idx.RefreshState(context.Background(), repo, at, sourcebridge.Source{Lookup: observed})
				}
				original, err := refresh()
				if err != nil {
					t.Fatal(err)
				}
				request := retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/job.runtime", Aspect: "runtime", Path: "status"}, Value: "running"})
				request.Limit = 1
				search := func(req retrieval.SearchRequest) (retrieval.SearchResult, error) {
					if scope == "dataset" {
						return idx.SearchDataset(context.Background(), lookup, pin, req)
					}
					revision := ""
					if req.Continuation != "" {
						revision = original.Revision
					}
					return idx.SearchStateAtRevisionContext(context.Background(), repo, at, revision, req)
				}
				first, err := search(request)
				if err != nil || len(first.Hits) != 1 || first.Continuation == "" {
					t.Fatalf("first observation page: %#v %v", first, err)
				}
				seen := map[objectmodel.ObjectID]bool{first.Hits[0].Object.Address.ObjectID: true}
				token := first.Continuation
				observed.Value = map[string]any{"status": "stopped"}
				observed.Basis.SourceRevision = "r2"
				observed.Basis.ObservedAt = "2026-10-21T00:01:00Z"
				replacement, err := refresh()
				if err != nil || replacement.Revision == original.Revision {
					t.Fatalf("replacement: %#v %v", replacement, err)
				}
				calls := observed.Calls
				current, err := search(request)
				if err != nil || len(current.Hits) != 0 || current.SearchView.ProjectionRevisions[repo.ID()] != replacement.Revision {
					t.Fatalf("new query used old observations: %#v %v", current, err)
				}
				if storage == "persistent" {
					if err := idx.Close(); err != nil {
						t.Fatal(err)
					}
					idx = NewIndexEngine(directory, opener)
				}
				for pageNumber := 0; token != ""; pageNumber++ {
					if pageNumber > 3 {
						t.Fatal("observation continuation did not advance")
					}
					nextRequest := request
					nextRequest.Continuation = token
					page, err := search(nextRequest)
					if err != nil || len(page.Hits) != 1 || page.SearchView.ProjectionRevisions[repo.ID()] != original.Revision {
						t.Fatalf("old observation was not retained: %#v %v", page, err)
					}
					hit := page.Hits[0]
					if seen[hit.Object.Address.ObjectID] || hit.Object.Value.(map[string]any)["runtime"].(map[string]any)["status"] != "running" || len(hit.Version.Observations) != 1 || hit.Version.Observations[0].Basis.SourceRevision != "r1" {
						t.Fatalf("mixed or repeated observation: %#v", hit)
					}
					seen[hit.Object.Address.ObjectID] = true
					token = page.Continuation
				}
				if len(seen) != 3 || observed.Calls != calls {
					t.Fatalf("continuation lost objects or called runtime: seen=%v calls=%d want=%d", seen, observed.Calls, calls)
				}
				bad := retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/job.runtime", Aspect: "runtime", Path: "status"}, Value: "stopped"})
				bad.Continuation = first.Continuation
				if _, err := search(bad); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
					t.Fatalf("changed query admitted: %v", err)
				}
				if storage == "anonymous" {
					temporary := idx.stateDirectory
					if temporary == "" || !idx.temporaryStateDirectory {
						t.Fatal("anonymous state has no owned lifetime")
					}
					if err := idx.Close(); err != nil {
						t.Fatal(err)
					}
					if _, err := os.Stat(temporary); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("anonymous state survived owner close: %v", err)
					}
				}
			})
		}
	}
}
