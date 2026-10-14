package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
)

func TestNativeWriterPreservesOmittedPathAndAllowsExplicitMove(t *testing.T) {
	for _, kind := range []objectmodel.AddressKind{objectmodel.KindEntity, objectmodel.KindAspect, objectmodel.KindRelation} {
		t.Run(string(kind), func(t *testing.T) {
			base := testkit.MakeRepository(t, "kr://test/native-path")
			native := &nativeSchemaRepository{ObjectRepository: base}
			registry := snapshot.NewRegistry()
			if err := registry.Add(native); err != nil {
				t.Fatal(err)
			}
			w, err := writer.NewWriter(registry, nil)
			if err != nil {
				t.Fatal(err)
			}
			address := objectmodel.Address{Kind: kind, ObjectID: "record/A"}
			if kind == objectmodel.KindAspect {
				address.AspectName = "body"
			}
			value := map[string]any{"body": "first"}
			if kind == objectmodel.KindRelation {
				value = map[string]any{
					"relationId": "record/A", "relationType": "links", "direction": "DIRECTED",
					"endpoints": []any{
						map[string]any{"role": "from", "objectRef": map[string]any{"repository": string(base.ID()), "object": "source"}},
						map[string]any{"role": "to", "objectRef": map[string]any{"repository": "kr://foreign/repo", "object": "target"}},
					},
				}
			}
			put := func(command, hint string) kernel.CommitID {
				t.Helper()
				head, err := base.Head(snapshot.DefaultRef)
				if err != nil {
					t.Fatal(err)
				}
				receipt, err := w.Commit(command, objectmodel.ChangeSet{
					TargetRepository: base.ID(), TargetRef: snapshot.DefaultRef,
					BaseCommit: head, ExpectedTargetCommit: head,
					Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: address, Value: value, PathHint: hint}},
				})
				if err != nil {
					t.Fatal(err)
				}
				return receipt.Result.CommitID
			}
			first := put("create", "public/custom.yaml")
			value["body"] = "updated"
			second := put("update", "")
			third := put("move", "archive/moved.yaml")
			for commit, want := range map[kernel.CommitID]string{first: "public/custom.yaml", second: "public/custom.yaml", third: "archive/moved.yaml"} {
				resolved, err := base.ResolveAddress(address, commit)
				if err != nil || resolved.PathHint != want {
					t.Fatalf("path at %s: got %q, want %q; error %v", commit, resolved.PathHint, want, err)
				}
			}
		})
	}
}
