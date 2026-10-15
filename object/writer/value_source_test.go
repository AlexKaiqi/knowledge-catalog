package writer_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

var valueSourceCommand atomic.Int64

func commandID() string { return fmt.Sprintf("value-source-%d", valueSourceCommand.Add(1)) }

func commitOps(t *testing.T, s testkit.Setup, base kernel.CommitID, ops ...objectmodel.Operation) (kernel.CommitID, error) {
	t.Helper()
	receipt, err := s.Writer.Commit(commandID(), objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base, Operations: ops,
	})
	if err != nil {
		return "", err
	}
	return receipt.Result.NewCommit, nil
}

func statsSchema() objectmodel.Operation {
	return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/table.stats"},
		Value: map[string]any{"entity": "Table", "aspect": "stats", "fields": map[string]any{"rowCount": map[string]any{"type": "number"}}}}
}

func TestWriterPublishesBindingUnitWithDescriptorRef(t *testing.T) {
	s := testkit.NewSetup(t, "")
	commit, err := commitOps(t, s, s.RootCommitID,
		statsSchema(),
		bindingfixture.PutDescriptor("resource/stats", bindingfixture.Descriptor("https://stats.example")),
		bindingfixture.PutAspect("table/orders", "stats", "schema/table.stats", "resource/stats"),
	)
	if err != nil {
		t.Fatal(err)
	}
	value, err := s.Reader.ReadAddress(s.RepositoryID, objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "stats"}, commit)
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Declarations) != 1 || !value.Declarations[0].ValueSource.IsBinding() || value.Declarations[0].ValueSource.Binding.DescriptorRef != "resource/stats" {
		t.Fatalf("binding declaration %#v", value.Declarations)
	}
	// A later commit may reference the descriptor already on the target basis.
	if _, err := commitOps(t, s, commit, bindingfixture.PutAspect("table/items", "stats", "schema/table.stats", "resource/stats")); err != nil {
		t.Fatalf("descriptor on the target basis must resolve: %v", err)
	}
}

func TestWriterRejectsInvalidValueSourceDeclarations(t *testing.T) {
	descriptor := bindingfixture.PutDescriptor("resource/stats", bindingfixture.Descriptor("https://stats.example"))
	notDescriptor := objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "resource/plain"}, Value: map[string]any{"name": "plain"}}
	withValue := bindingfixture.PutAspect("table/orders", "stats", "schema/table.stats", "resource/stats")
	withValue.Value = map[string]any{"rowCount": 1}
	onEntity := bindingfixture.Bind(objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "table/orders"}, "", "resource/stats")
	onSchema := bindingfixture.Bind(objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/table.stats"}, "", "resource/stats")
	nested := objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "stats"}, SchemaRef: "schema/table.stats",
		Value: map[string]any{"valueSource": map[string]any{"kind": "binding"}}}
	credential := bindingfixture.Descriptor("https://stats.example")
	credential["headers"] = map[string]any{"Authorization": "Bearer secret"}
	cases := map[string]struct {
		ops  []objectmodel.Operation
		code kernel.ErrorCode
	}{
		"binding carries a value":   {[]objectmodel.Operation{statsSchema(), descriptor, withValue}, kernel.ErrUsageInvalid},
		"binding on entity":         {[]objectmodel.Operation{descriptor, onEntity}, kernel.ErrUsageInvalid},
		"binding on schema":         {[]objectmodel.Operation{descriptor, onSchema}, kernel.ErrUsageInvalid},
		"valueSource inside a body": {[]objectmodel.Operation{statsSchema(), nested}, kernel.ErrUsageInvalid},
		"unresolved descriptorRef":  {[]objectmodel.Operation{statsSchema(), bindingfixture.PutAspect("table/orders", "stats", "schema/table.stats", "resource/missing")}, kernel.ErrObjectRefUnresolved},
		"descriptorRef to plain object": {[]objectmodel.Operation{statsSchema(), notDescriptor,
			bindingfixture.PutAspect("table/orders", "stats", "schema/table.stats", "resource/plain")}, kernel.ErrUsageInvalid},
		"descriptor with credentials": {[]objectmodel.Operation{bindingfixture.PutDescriptor("resource/secret", credential)}, kernel.ErrUsageInvalid},
		"aspect and member both bound": {[]objectmodel.Operation{statsSchema(), descriptor,
			bindingfixture.PutAspect("table/orders", "stats", "", "resource/stats"),
			bindingfixture.PutMember("table/orders", "stats", "current", "", "resource/stats")}, kernel.ErrObjectIDConflict},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := testkit.NewSetup(t, "")
			_, err := commitOps(t, s, s.RootCommitID, tc.ops...)
			testkit.ExpectCode(t, err, tc.code)
			if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != s.RootCommitID {
				t.Fatalf("rejected declaration moved HEAD to %s", got)
			}
		})
	}
}

func TestWriterRejectsRemovingDescriptorStillInBatchUse(t *testing.T) {
	s := testkit.NewSetup(t, "")
	commit, err := commitOps(t, s, s.RootCommitID, bindingfixture.PutDescriptor("resource/stats", bindingfixture.Descriptor("https://stats.example")))
	if err != nil {
		t.Fatal(err)
	}
	_, err = commitOps(t, s, commit,
		objectmodel.Operation{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "resource/stats"}},
		bindingfixture.PutAspect("table/orders", "stats", "", "resource/stats"))
	testkit.ExpectCode(t, err, kernel.ErrObjectRefUnresolved)
}

func TestWriterResolvesDescriptorRefAcrossRepositories(t *testing.T) {
	s := testkit.NewSetup(t, "kr://acme/public/core")
	shared := testkit.MakeRepository(t, "kr://acme/public/resources")
	if err := s.Store.Add(shared); err != nil {
		t.Fatal(err)
	}
	root := testkit.MustHead(t, shared, snapshot.DefaultRef)
	if _, err := s.Writer.Commit(commandID(), objectmodel.CommitChangeSet{TargetRepository: shared.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{bindingfixture.PutDescriptor("resource/stats", bindingfixture.Descriptor("https://stats.example"))}}); err != nil {
		t.Fatal(err)
	}
	if _, err := commitOps(t, s, s.RootCommitID, bindingfixture.PutAspect("table/orders", "stats", "", "resource/stats")); err != nil {
		t.Fatalf("descriptor held by another repository must resolve by ID: %v", err)
	}

	again := testkit.MakeRepository(t, "kr://acme/public/resources-copy")
	if err := s.Store.Add(again); err != nil {
		t.Fatal(err)
	}
	copyRoot := testkit.MustHead(t, again, snapshot.DefaultRef)
	if _, err := s.Writer.Commit(commandID(), objectmodel.CommitChangeSet{TargetRepository: again.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: copyRoot, ExpectedTargetCommit: copyRoot,
		Operations: []objectmodel.Operation{bindingfixture.PutDescriptor("resource/stats", bindingfixture.Descriptor("https://other.example"))}}); err != nil {
		t.Fatal(err)
	}
	head := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
	_, err := commitOps(t, s, head, bindingfixture.PutAspect("table/items", "stats", "", "resource/stats"))
	testkit.ExpectCode(t, err, kernel.ErrObjectIDConflict)
}
