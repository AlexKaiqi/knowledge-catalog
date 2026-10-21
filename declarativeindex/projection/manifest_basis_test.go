package projection_test

import (
	"context"
	"testing"

	indexcli "kc/declarativeindex/capability"
	"kc/declarativeindex/internal/testfixture"
	index "kc/declarativeindex/projection"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestManifestChangeInvalidatesPhysicalBasisWithIdenticalLogicalSupport(t *testing.T) {
	s := testkit.NewSetup(t, "kr://manifest/basis")
	_, err := s.Writer.Commit("manifest-seed", objectmodel.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}},
		testkit.IndexText("schema/article", "body"),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/one"}, SchemaRef: "schema/article", Value: map[string]any{"body": "runbook"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	commit := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
	engine := &testfixture.StateEngine{}
	idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	manifest := indexcli.DefaultManifest()
	idx.SetDeclarationManifest(manifest.Check, manifest.Describe().Digest)
	first, err := idx.Ensure(context.Background(), s.Repo, commit)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Limits.MaxTokens-- // Same member is still supported, but the instance interpretation changed.
	idx.SetDeclarationManifest(manifest.Check, manifest.Describe().Digest)
	if _, err := idx.CheckSearchProjectionAt(s.Repo, commit); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Errorf("old physical basis remains ready after manifest changed: %v", err)
	}
	second, err := idx.Ensure(context.Background(), s.Repo, commit)
	if err != nil {
		t.Fatal(err)
	}
	if second.Mode != ports.IndexModeRebuild || second.AccessDigest != first.AccessDigest || second.PhysicalDigest == first.PhysicalDigest {
		t.Fatalf("manifest change lost or altered logical declaration: first=%#v second=%#v", first, second)
	}
	if _, err := idx.CheckSearchProjectionAt(s.Repo, commit); err != nil {
		t.Fatal(err)
	}
	third, err := idx.Ensure(context.Background(), s.Repo, commit)
	if err != nil || third.Mode != ports.IndexModeReady || third.PhysicalDigest != second.PhysicalDigest {
		t.Fatalf("unchanged instance rebuilt again: %#v %v", third, err)
	}
}
