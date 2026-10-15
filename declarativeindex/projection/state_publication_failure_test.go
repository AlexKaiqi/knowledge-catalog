package projection

import (
	"context"
	"errors"
	"kc/declarativeindex/internal/testfixture"
	ports "kc/discovery/projection"
	"path/filepath"
	"reflect"
	"testing"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"
	"kc/snapshot"
)

type statePublicationFailureEngine struct {
	testfixture.StateEngine
	commitErr error
	commits   int
	candidate ports.Meta
	appended  int
}

func (e *statePublicationFailureEngine) BeginRebuild(meta ports.Meta) (ports.RebuildSession, error) {
	e.candidate, e.appended = meta, 0
	session, err := e.StateEngine.BeginRebuild(meta)
	if err != nil {
		return nil, err
	}
	return &statePublicationFailureSession{RebuildSession: session, engine: e}, nil
}

type statePublicationFailureSession struct {
	ports.RebuildSession
	engine *statePublicationFailureEngine
}

func (s *statePublicationFailureSession) Append(docs []ports.CompiledDoc) error {
	s.engine.appended += len(docs)
	return s.RebuildSession.Append(docs)
}

func (s *statePublicationFailureSession) Commit() error {
	s.engine.commits++
	if s.engine.commitErr != nil {
		return s.RebuildSession.Abort(s.engine.commitErr)
	}
	return s.RebuildSession.Commit()
}

func TestStatePublicationFailurePreservesOriginalObservationUntilRetry(t *testing.T) {
	for _, mode := range []string{"rebuild", "incremental"} {
		t.Run(mode, func(t *testing.T) {
			repo, commit, address := testfixture.StateProjection(t)
			engine := &statePublicationFailureEngine{}
			idx := NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
			t.Cleanup(func() { _ = idx.Close() })
			lookup := &testfixture.StateLookup{
				Value: map[string]any{"status": "running"},
				Basis: objectmodel.ObservationBasis{
					BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable,
					SourceRevision: "r1", ObservedAt: "2026-08-27T00:00:00Z",
				},
			}
			source := sourcebridge.Source{Lookup: lookup}
			original, err := idx.RefreshState(context.Background(), repo, commit, source)
			if err != nil {
				t.Fatal(err)
			}
			query := func(status string) retrieval.SearchRequest {
				return retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ,
					Field: &retrieval.FieldRef{Schema: "schema/job.runtime", Aspect: "runtime", Path: "status"}, Value: status,
				})
			}
			before, err := idx.SearchStateAtRevision(repo, commit, original.Revision, query("running"))
			if err != nil || before.Completeness != retrieval.CompletenessComplete || len(before.Hits) != 1 ||
				before.SearchView.ProjectionRevisions[repo.ID()] != original.Revision || len(before.Hits[0].Version.Observations) != 1 ||
				before.Hits[0].Version.Observations[0].Basis.SourceRevision != "r1" {
				t.Fatalf("initial observation was not served at r1: result=%#v error=%v", before, err)
			}
			oldStore := idx.states[stateStoreKey(repo.ID(), commit)].store.file
			lookup.Value = map[string]any{"status": "stopped"}
			lookup.Basis.SourceRevision = "r2"
			lookup.Basis.ObservedAt = "2026-08-27T00:01:00Z"
			problem := kernel.Fail(kernel.ErrTemporaryUnavailable, "candidate generation publication failed")
			engine.commitErr = problem
			refresh := func() (ports.StateSync, error) {
				if mode == "incremental" {
					return idx.RefreshStateObjects(context.Background(), repo, commit, source, []objectmodel.ObjectID{address.ObjectID})
				}
				return idx.RefreshState(context.Background(), repo, commit, source)
			}
			failed, err := refresh()
			if !errors.Is(err, problem) || kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || !reflect.DeepEqual(failed, ports.StateSync{}) ||
				// Two documents: the job and the ResourceDescriptor it binds.
				engine.commits != 2 || engine.appended != 2 || engine.candidate.Revision == original.Revision || lookup.Calls != 2 {
				t.Fatalf("failure did not reach actual candidate publication: sync=%#v error=%v commits=%d appended=%d lookups=%d", failed, err, engine.commits, engine.appended, lookup.Calls)
			}
			failedRevision := engine.candidate.Revision
			if revision, ok := idx.StateView(repo.ID(), commit); !ok || revision != original.Revision {
				t.Fatalf("failed candidate became the public revision: %s %v", revision, ok)
			}
			after, err := idx.SearchStateAtRevision(repo, commit, original.Revision, query("running"))
			// Timing is diagnostic; compare the full returned object, version,
			// observation, provider evidence, fixed view and completeness instead.
			before.Stats, after.Stats = retrieval.SearchExecutionStats{}, retrieval.SearchExecutionStats{}
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("failed publication changed the original result: before=%#v after=%#v error=%v", before, after, err)
			}
			unpublished, err := idx.SearchStateAt(repo, commit, query("stopped"))
			if err != nil || unpublished.Completeness != retrieval.CompletenessComplete || len(unpublished.Hits) != 0 {
				t.Fatalf("saved candidate observation leaked into current search: result=%#v error=%v", unpublished, err)
			}
			if _, err := idx.SearchStateAtRevision(repo, commit, failedRevision, query("stopped")); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("failed candidate revision became addressable: %v", err)
			}
			files, err := filepath.Glob(filepath.Join(filepath.Dir(oldStore), "*.db"))
			if err != nil || !reflect.DeepEqual(files, []string{oldStore}) {
				t.Fatalf("failed candidate leaked storage or removed serving observation: files=%v error=%v", files, err)
			}
			engine.commitErr = nil
			retried, err := refresh()
			if err != nil || retried.Revision != failedRevision || retried.Revision == original.Revision || engine.commits != 3 {
				t.Fatalf("retry did not publish the new observation: sync=%#v error=%v commits=%d", retried, err, engine.commits)
			}
			current, err := idx.SearchStateAtRevision(repo, commit, retried.Revision, query("stopped"))
			if err != nil || current.Completeness != retrieval.CompletenessComplete || len(current.Hits) != 1 ||
				current.Hits[0].Object.Value.(map[string]any)["runtime"].(map[string]any)["status"] != "stopped" ||
				current.Hits[0].Version.Observations[0].Basis.SourceRevision != "r2" || current.SearchView.ProjectionRevisions[repo.ID()] != retried.Revision {
				t.Fatalf("retry mixed the new candidate and original observation: result=%#v error=%v", current, err)
			}
			if _, err := idx.SearchStateAtRevision(repo, commit, original.Revision, query("running")); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("retired revision silently read the newly published observation: %v", err)
			}
			if head := testkit.MustHead(t, repo, snapshot.DefaultRef); head != commit {
				t.Fatalf("publication or retry changed the declaration repository: head=%s want=%s", head, commit)
			}
		})
	}
}
