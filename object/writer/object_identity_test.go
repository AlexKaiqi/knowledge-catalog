package writer_test

import (
	"fmt"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
)

// Every Writer surface and both provider paths share the object identity
// contract. A rejected declaration must leave the accepted version readable.
func TestWriterRejectsEmptyBusinessObjectIdentityWithoutAdvancingVersion(t *testing.T) {
	for _, native := range []bool{false, true} {
		provider := "tree"
		if native {
			provider = "native"
		}
		for _, surface := range []string{"commit", "intent", "proposal"} {
			t.Run(provider+"/"+surface, func(t *testing.T) {
				base := testkit.MakeRepository(t, "kr://test/object-identity")
				registry := snapshot.NewRegistry()
				var store snapshot.Store = base.Snapshot()
				if native {
					store = &nativeSchemaRepository{KnowledgeRepository: base}
				}
				if err := registry.Add(store); err != nil {
					t.Fatal(err)
				}
				w, err := writer.NewWriter(registry, nil)
				if err != nil {
					t.Fatal(err)
				}
				head := testkit.MustHead(t, base, snapshot.DefaultRef)
				accepted, err := w.Commit("accepted", objectmodel.ChangeSet{TargetRepository: base.ID(), TargetRef: snapshot.DefaultRef,
					BaseCommit: head, ExpectedTargetCommit: head, Operations: testkit.PutEntity("image/kept", "accepted value", "media/kept.yaml")})
				if err != nil {
					t.Fatal(err)
				}
				head = accepted.Result.CommitID
				for _, kind := range []objectmodel.AddressKind{objectmodel.KindEntity, objectmodel.KindAspect, objectmodel.KindRecord, objectmodel.KindMember, objectmodel.KindRelation} {
					for number, id := range []objectmodel.ObjectID{"", " \t\n"} {
						for _, op := range []objectmodel.OpKind{objectmodel.OpPut, objectmodel.OpRemove} {
							address := objectmodel.Address{Kind: kind, ObjectID: id}
							if kind == objectmodel.KindAspect || kind == objectmodel.KindRecord || kind == objectmodel.KindMember {
								address.AspectName = "description"
							}
							if kind == objectmodel.KindMember {
								address.MemberKey = "one"
							}
							operations := []objectmodel.Operation{{Op: op, Address: address, Value: "rejected value"}}
							cs := objectmodel.ChangeSet{TargetRepository: base.ID(), TargetRef: snapshot.DefaultRef,
								BaseCommit: head, ExpectedTargetCommit: head, Operations: operations}
							command := fmt.Sprintf("rejected-%s-%d-%s", kind, number, op)
							switch surface {
							case "commit":
								_, err = w.Commit(command, cs)
							case "intent":
								_, err = w.CommitIntent(command, writer.CommitIntent{TargetRepository: base.ID(), TargetRef: snapshot.DefaultRef, Operations: operations})
							case "proposal":
								_, err = w.Propose(command, writer.ProposeIntent{TargetRepository: base.ID(),
									CandidateRef: "refs/heads/candidates/" + command, BaseCommit: head, Operations: operations})
							}
							if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
								t.Errorf("%s %s with identity %q: got %v, want USAGE_INVALID", op, kind, id, err)
							}
							if got := testkit.MustHead(t, base, snapshot.DefaultRef); got != head {
								t.Fatalf("rejected identity advanced HEAD: %s -> %s", head, got)
							}
						}
					}
				}
				value, err := base.Read("image/kept", head)
				if err != nil || value.Value != "accepted value" {
					t.Fatalf("rejection changed accepted object: %#v, %v", value, err)
				}
			})
		}
	}
}
