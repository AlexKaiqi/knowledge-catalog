package projection_test

import (
	"context"
	"strings"
	"testing"

	"kc/declarativeindex/capability"
	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

// publishedFixture is one Dataset source holding a Schema, a selected
// declaration, and two Article objects. It is the smallest world that can answer
// "which revision serves this query", so visibility evidence does not depend on
// a deployment.
type publishedFixture struct {
	repo   *testkit.ObjectRepository
	before kernel.CommitID
	after  kernel.CommitID
}

func newPublishedFixture(t *testing.T) publishedFixture {
	t.Helper()
	repo := testkit.MakeRepository(t, "kr://index/published")
	commit := func(base kernel.CommitID, ops ...objectmodel.Operation) kernel.CommitID {
		t.Helper()
		at, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		return at
	}
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	before := commit(root,
		objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"},
			Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}},
		testkit.IndexText("schema/article", "body"),
		objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/one"},
			Value: map[string]any{"body": "original runbook"}, SchemaRef: "schema/article"},
	)
	after := commit(before,
		objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/two"},
			Value: map[string]any{"body": "fresh addition"}, SchemaRef: "schema/article"},
	)
	return publishedFixture{repo: repo, before: before, after: after}
}

func (f publishedFixture) pin(setID string, revision int, at kernel.CommitID) reader.DatasetPin {
	return reader.DatasetPin{SetID: setID, Revision: revision,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{f.repo.ID(): at},
		Items:        []reader.DatasetItem{{Repository: f.repo.ID(), Commit: at, Kind: "prefix"}}}
}

func (f publishedFixture) lookup() reader.MemberLookup {
	return func(kernel.RepositoryID) (objectmodel.Repository, error) { return f.repo, nil }
}

func newPublishedIndex(t *testing.T) *index.Index {
	t.Helper()
	engines := map[kernel.RepositoryID]*datasetTestEngine{}
	idx := index.NewIndexEngine(t.TempDir(), func(_ string, id kernel.RepositoryID) (ports.Engine, error) {
		eng := engines[id]
		if eng == nil {
			eng = &datasetTestEngine{}
			engines[id] = eng
		}
		return eng, nil
	})
	t.Cleanup(func() { _ = idx.Close() })
	manifest := capability.DefaultManifest()
	idx.SetDeclarationManifest(manifest.Check, manifest.Describe().Digest)
	return idx
}

func searchBodies(t *testing.T, idx *index.Index, lookup reader.MemberLookup, pin reader.DatasetPin, query string) []string {
	t.Helper()
	result, err := idx.SearchDataset(context.Background(), lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH(query)))
	if err != nil {
		t.Fatalf("search %q on revision %d: %v", query, pin.Revision, err)
	}
	if result.Dataset == nil || result.Dataset.ID != pin.SetID || result.Dataset.Revision != pin.Revision {
		t.Fatalf("search must report the revision it actually scored: %#v", result.Dataset)
	}
	bodies := make([]string, 0, len(result.Hits))
	for _, hit := range result.Hits {
		bodies = append(bodies, readBody(t, hit.Object))
	}
	return bodies
}

func readBody(t *testing.T, value objectmodel.ObjectValue) string {
	t.Helper()
	body, _ := value.Value.(map[string]any)["body"].(string)
	return body
}

// TestServingRevisionStaysOnTheLastReadyOneWhileTheNextIsUnbuilt is the
// visibility half of "accept first, build later": a newly accepted revision is
// not searchable until its corpus exists, and until then the previous ready
// revision keeps serving instead of the query failing or drifting.
func TestServingRevisionStaysOnTheLastReadyOneWhileTheNextIsUnbuilt(t *testing.T) {
	fixture := newPublishedFixture(t)
	idx := newPublishedIndex(t)
	lookup := fixture.lookup()
	first, second := fixture.pin("articles", 1, fixture.before), fixture.pin("articles", 2, fixture.after)
	if _, err := idx.PrepareDataset(context.Background(), lookup, first); err != nil {
		t.Fatal(err)
	}
	if got := searchBodies(t, idx, lookup, first, "runbook"); len(got) != 1 || got[0] != "original runbook" {
		t.Fatalf("revision 1 corpus: %#v", got)
	}
	// Revision 2 is accepted but never built. Its own query must fail as "not
	// searchable" rather than answer with an empty result set, which a caller
	// could mistake for "no matches".
	if _, err := idx.SearchDataset(context.Background(), lookup, second, retrieval.SearchOf(retrieval.SearchMATCH("fresh"))); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unbuilt revision must answer not searchable, got %v", err)
	}
	if !strings.Contains(errText(idx, lookup, second), "not prepared") {
		t.Fatalf("unbuilt revision must say it is not prepared: %s", errText(idx, lookup, second))
	}
	// Revision 1 still serves, and the object added after its pin is invisible.
	if got := searchBodies(t, idx, lookup, first, "fresh"); len(got) != 0 {
		t.Fatalf("revision 1 must not see objects added after its pin: %#v", got)
	}

	if _, err := idx.PrepareDataset(context.Background(), lookup, second); err != nil {
		t.Fatal(err)
	}
	if got := searchBodies(t, idx, lookup, second, "fresh"); len(got) != 1 || got[0] != "fresh addition" {
		t.Fatalf("revision 2 corpus: %#v", got)
	}
	// Revision 1 is still answerable from its own frozen corpus after revision 2
	// was built: versions do not cannibalise each other.
	if got := searchBodies(t, idx, lookup, first, "runbook"); len(got) != 1 || got[0] != "original runbook" {
		t.Fatalf("revision 1 corpus changed after building revision 2: %#v", got)
	}
}

func errText(idx *index.Index, lookup reader.MemberLookup, pin reader.DatasetPin) string {
	_, err := idx.SearchDataset(context.Background(), lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("anything")))
	if err == nil {
		return ""
	}
	return err.Error()
}

// TestRevisionCorpusIsFrozenAtItsPin pins attribution: a revision scores the
// corpus of the commits it pinned, so a source that moves afterwards changes
// neither its hits nor the revision the response reports.
func TestRevisionCorpusIsFrozenAtItsPin(t *testing.T) {
	fixture := newPublishedFixture(t)
	idx := newPublishedIndex(t)
	lookup := fixture.lookup()
	pin := fixture.pin("frozen", 1, fixture.before)
	if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
		t.Fatal(err)
	}
	before := searchBodies(t, idx, lookup, pin, "runbook")
	if len(before) != 1 {
		t.Fatalf("pinned corpus: %#v", before)
	}
	// The source moves on: a new object and an edit to the pinned one.
	head, err := fixture.repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: fixture.repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/one"},
			Value: map[string]any{"body": "rewritten runbook"}, SchemaRef: "schema/article"},
	}}); err != nil {
		t.Fatal(err)
	}
	after := searchBodies(t, idx, lookup, pin, "runbook")
	if len(after) != 1 || after[0] != "original runbook" {
		t.Fatalf("pinned revision followed the source after its pin: %#v", after)
	}
}

// TestTwoRevisionsOfOneDatasetKeepTheirOwnDocuments proves conversations in
// parallel builds do not cross: each revision answers only its own objects, and
// the answer reports the revision it scored.
func TestTwoRevisionsOfOneDatasetKeepTheirOwnDocuments(t *testing.T) {
	fixture := newPublishedFixture(t)
	idx := newPublishedIndex(t)
	lookup := fixture.lookup()
	first, second := fixture.pin("parallel", 1, fixture.before), fixture.pin("parallel", 2, fixture.after)
	if _, err := idx.PrepareDataset(context.Background(), lookup, first); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.PrepareDataset(context.Background(), lookup, second); err != nil {
		t.Fatal(err)
	}
	one, err := idx.DatasetReadiness(context.Background(), lookup, first)
	if err != nil {
		t.Fatal(err)
	}
	two, err := idx.DatasetReadiness(context.Background(), lookup, second)
	if err != nil {
		t.Fatal(err)
	}
	// A revision's document count covers everything its pin compiled, so the
	// absolute numbers depend on the fixture; what must hold is that the later
	// revision owns exactly the one object it added and nothing more, which is
	// only true if the two corpora were built and stored apart.
	if !one.Ready || !two.Ready || two.ObjectCount != one.ObjectCount+1 {
		t.Fatalf("each revision must own its own document count: %#v %#v", one, two)
	}
	if got := searchBodies(t, idx, lookup, first, "runbook"); len(got) != 1 || got[0] != "original runbook" {
		t.Fatalf("revision 1 corpus collided with revision 2: %#v", got)
	}
	if got := searchBodies(t, idx, lookup, second, "fresh"); len(got) != 1 || got[0] != "fresh addition" {
		t.Fatalf("revision 2 corpus collided with revision 1: %#v", got)
	}
}

// TestUnconfiguredIndexAnswersCapabilityUnsatisfied keeps "no index instance" a
// distinct answer from "nothing is being built", so an operator is never told a
// deployment is idle when it simply has no index configured.
func TestUnconfiguredIndexAnswersCapabilityUnsatisfied(t *testing.T) {
	fixture := newPublishedFixture(t)
	idx := index.NewIndexEngine(t.TempDir(), nil)
	t.Cleanup(func() { _ = idx.Close() })
	pin := fixture.pin("absent", 1, fixture.before)
	// The request itself is well formed, so the only reason to refuse it is the
	// missing index: an unconfigured deployment must say so instead of reporting
	// a not-built revision, which would read as "waiting to be built".
	ready, err := idx.DatasetReadiness(context.Background(), fixture.lookup(), pin)
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unconfigured index must answer capability unsatisfied, got %v (%#v)", err, ready)
	}
	if _, err := idx.PrepareDataset(context.Background(), fixture.lookup(), pin); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unconfigured index must refuse to build, got %v", err)
	}
}
