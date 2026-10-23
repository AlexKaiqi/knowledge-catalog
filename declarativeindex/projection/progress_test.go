package projection

import (
	"context"
	"testing"

	"kc/declarativeindex/internal/testfixture"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

// progressFixture seeds one Dataset source holding two Article objects that a
// selected declaration covers, one Article whose Schema has no selected
// declaration, and one object the Dataset scope leaves out entirely. Builds of
// this pin must visit every object in scope but publish only the covered ones,
// which is what makes the object denominator and the document count diverge
// (doc/design/discovery.md#索引维护).
type progressFixture struct {
	repo   *testkit.ObjectRepository
	commit kernel.CommitID
	pin    reader.DatasetPin
}

func newProgressFixture(t *testing.T) progressFixture {
	t.Helper()
	repo := testkit.MakeRepository(t, "kr://index/progress")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"},
				Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}},
			testkit.IndexText("schema/article", "body"),
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/a"},
				Value: map[string]any{"body": "alpha"}, SchemaRef: "schema/article"},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/b"},
				Value: map[string]any{"body": "beta"}, SchemaRef: "schema/article"},
			// schema/other is selected but declares nothing, so other/x is
			// visited by the build and contributes no document.
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/other"},
				Value: map[string]any{"entity": "Other", "fields": map[string]any{"note": map[string]any{"type": "string"}}}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "other/x"},
				Value: map[string]any{"note": "no selected declaration"}, SchemaRef: "schema/other"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return progressFixture{repo: repo, commit: commit, pin: reader.DatasetPin{
		Temporary: true, SetID: "progress-set", Revision: 1,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit},
		Items:        []reader.DatasetItem{{Repository: repo.ID(), Commit: commit, Kind: "prefix"}},
	}}
}

func (f progressFixture) lookup() reader.MemberLookup {
	return func(kernel.RepositoryID) (objectmodel.Repository, error) { return f.repo, nil }
}

// samplingEngine records the in-flight progress this instance reports while the
// build is actually running. Reading progress after the build would prove
// nothing: the entry is removed when the build finishes.
type samplingEngine struct {
	*testfixture.StateEngine

	observe func()
	samples []ports.IndexCatchUp
}

func (e *samplingEngine) Rebuild(docs []ports.CompiledDoc, meta ports.Meta) error {
	if e.observe != nil {
		e.observe()
	}
	return e.StateEngine.Rebuild(docs, meta)
}

// TestBuildProgressCountsVisitedObjectsIncludingUnindexableOnes pins the
// denominator of a build: every object the build visits counts toward total,
// objects without a selected declaration publish no document but still count as
// finished work, and the reported object count is documents only.
func TestBuildProgressCountsVisitedObjectsIncludingUnindexableOnes(t *testing.T) {
	fixture := newProgressFixture(t)
	engine := &samplingEngine{StateEngine: &testfixture.StateEngine{}}
	idx := NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	engine.observe = func() { engine.samples = append(engine.samples, idx.CatchUpProgress()...) }

	sync, err := idx.PrepareDataset(context.Background(), fixture.lookup(), fixture.pin)
	if err != nil {
		t.Fatal(err)
	}
	if len(engine.samples) != 1 {
		t.Fatalf("expected exactly one in-flight sample, got %#v", engine.samples)
	}
	inFlight := engine.samples[0]
	// Four objects are compiled (two Articles, the declaration member and the
	// uncovered object is still visited), and the Schema objects are visited
	// without documents. The denominator counts visits, not documents.
	if inFlight.Total <= sync.ObjectCount {
		t.Fatalf("total %d must exceed the document count %d: unindexable objects were dropped from the denominator", inFlight.Total, sync.ObjectCount)
	}
	if inFlight.Done != inFlight.Total {
		t.Fatalf("progress must finish every visited object: done=%d total=%d", inFlight.Done, inFlight.Total)
	}
	if inFlight.Dataset != fixture.pin.SetID || inFlight.Revision != fixture.pin.Revision {
		t.Fatalf("progress must be attributable to the revision being built, got %#v", inFlight)
	}
	if inFlight.Repository == "" {
		t.Fatal("progress must name the building repository")
	}
	if ready := idx.CatchUpProgress(); len(ready) != 0 {
		t.Fatalf("finished build must leave no progress behind: %#v", ready)
	}
}

// TestDatasetReadinessAnswersWithoutBuildingAnything keeps the narrow question
// narrow: asking whether a revision is searchable must not compile a corpus, so
// a revision that was never built answers false instead of becoming ready as a
// side effect of the question.
func TestDatasetReadinessAnswersWithoutBuildingAnything(t *testing.T) {
	fixture := newProgressFixture(t)
	engine := &samplingEngine{StateEngine: &testfixture.StateEngine{}}
	idx := NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	builds := 0
	engine.observe = func() { builds++ }

	before, err := idx.DatasetReadiness(context.Background(), fixture.lookup(), fixture.pin)
	if err != nil {
		t.Fatal(err)
	}
	if before.Ready || before.ObjectCount != 0 {
		t.Fatalf("never-built revision must answer not ready without a count: %#v", before)
	}
	if builds != 0 {
		t.Fatalf("readiness alone built a corpus (%d builds)", builds)
	}
	if before.Dataset != fixture.pin.SetID || before.Revision != fixture.pin.Revision {
		t.Fatalf("readiness must echo the revision it answered: %#v", before)
	}

	if _, err := idx.PrepareDataset(context.Background(), fixture.lookup(), fixture.pin); err != nil {
		t.Fatal(err)
	}
	after, err := idx.DatasetReadiness(context.Background(), fixture.lookup(), fixture.pin)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Ready || after.ObjectCount == 0 {
		t.Fatalf("built revision must answer ready with its document count: %#v", after)
	}
	// One build total: the second readiness answer reused the built corpus.
	if builds != 1 {
		t.Fatalf("readiness rebuilt the corpus: %d builds", builds)
	}
}

// TestDatasetDiagnosisSeparatesStagesFromReadiness pins the second question:
// diagnosis names members and their stage, so an operator can tell "not built
// yet" from "failed" from "ready", which the narrow readiness answer cannot.
func TestDatasetDiagnosisSeparatesStagesFromReadiness(t *testing.T) {
	fixture := newProgressFixture(t)
	engine := &testfixture.StateEngine{}
	idx := NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })

	notBuilt, err := idx.DatasetDiagnosis(context.Background(), fixture.lookup(), fixture.pin)
	if err != nil {
		t.Fatal(err)
	}
	if notBuilt.Ready {
		t.Fatalf("never-built revision must not diagnose as ready: %#v", notBuilt)
	}
	if len(notBuilt.Members) != 1 || notBuilt.Members[0].Stage != ports.IndexStageNotBuilt {
		t.Fatalf("diagnosis must name each member and its stage: %#v", notBuilt.Members)
	}
	if notBuilt.Members[0].BasisCommit != fixture.commit {
		t.Fatalf("member basis must be the pinned source commit %s: %#v", fixture.commit, notBuilt.Members[0])
	}

	if _, err := idx.PrepareDataset(context.Background(), fixture.lookup(), fixture.pin); err != nil {
		t.Fatal(err)
	}
	ready, err := idx.DatasetDiagnosis(context.Background(), fixture.lookup(), fixture.pin)
	if err != nil {
		t.Fatal(err)
	}
	if !ready.Ready || ready.Members[0].Stage != ports.IndexStageReady {
		t.Fatalf("built revision must diagnose as ready: %#v", ready)
	}
	if ready.Basis == nil || ready.Basis.ObjectCount == 0 || ready.Basis.BasisCommit == "" {
		t.Fatalf("diagnosis must report the frozen corpus basis: %#v", ready.Basis)
	}
	// The corpus basis is the revision's derived space, not a source HEAD: the
	// pin already fixed which source commit each member contributes, so the
	// basis only has to identify the frozen corpus.
	if ready.Basis.BasisCommit == fixture.commit {
		t.Fatalf("corpus basis must not be mistaken for a source commit: %#v", ready.Basis)
	}
	if ready.Members[0].BasisCommit != fixture.commit {
		t.Fatalf("member basis must stay the pinned source commit %s: %#v", fixture.commit, ready.Members[0])
	}
}
