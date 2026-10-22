package liveaccess_test

import (
	"reflect"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func TestDescriptorDependencyFreezeUsesOneVersionForEveryReference(t *testing.T) {
	for _, change := range []string{"remove first descriptor", "modify first descriptor"} {
		t.Run(change, func(t *testing.T) {
			f := newDescriptorDependencyFixture(t)
			first, second := objectmodel.ObjectID("ops/warehouse-stats"), objectmodel.ObjectID("ops/zz-health")
			secondOrigin := "https://health-origin.example"
			published, err := f.writer.Commit("publish-second-descriptor", objectmodel.ChangeSet{
				TargetRepository: f.ops, TargetRef: snapshot.DefaultRef, BaseCommit: f.opsAt, ExpectedTargetCommit: f.opsAt,
				Operations: []objectmodel.Operation{bindingfixture.PutDescriptor(second, bindingfixture.Descriptor(secondOrigin))},
			})
			if err != nil {
				t.Fatal(err)
			}
			f.opsAt = published.Result.NewCommit
			bound, err := f.writer.Commit("bind-second-descriptor", objectmodel.ChangeSet{
				TargetRepository: f.table, TargetRef: snapshot.DefaultRef, BaseCommit: f.tableAt, ExpectedTargetCommit: f.tableAt,
				Operations: []objectmodel.Operation{
					{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/table.live-health"}, Value: map[string]any{
						"entity": "Table", "aspect": "live-health", "fields": map[string]any{"healthy": map[string]any{"type": "boolean"}},
					}},
					bindingfixture.PutAspect("table/shop.orders", "live-health", "schema/table.live-health", second),
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			f.tableAt = bound.Result.NewCommit

			// Also visit a candidate that holds neither reference. Its HEAD must
			// be fixed once even though it never becomes a dependency holder.
			absent := testkit.MakeTreeStore(t, "kr://dependency/zz-absent")
			if err := f.registry.Add(absent); err != nil {
				t.Fatal(err)
			}
			ops, err := f.lookup(f.ops)
			if err != nil {
				t.Fatal(err)
			}
			missing, err := f.lookup(absent.ID())
			if err != nil {
				t.Fatal(err)
			}
			absentAt := testkit.MustHead(t, absent, snapshot.DefaultRef)
			moving := &descriptorDependencyMovingRepository{Repository: ops, trigger: first}
			nonholder := &descriptorDependencyMovingRepository{Repository: missing}
			var advanced kernel.CommitID
			moving.afterRead = func() {
				op := objectmodel.Operation{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: first}}
				if change == "modify first descriptor" {
					op = bindingfixture.PutDescriptor(first, bindingfixture.Descriptor("https://changed-origin.example"))
				}
				moved, err := f.writer.Commit("advance-between-descriptor-reads", objectmodel.ChangeSet{
					TargetRepository: f.ops, TargetRef: snapshot.DefaultRef, BaseCommit: f.opsAt, ExpectedTargetCommit: f.opsAt,
					Operations: []objectmodel.Operation{op},
				})
				if err != nil {
					t.Fatal(err)
				}
				advanced = moved.Result.NewCommit
			}
			lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
				switch id {
				case f.ops:
					return moving, nil
				case absent.ID():
					return nonholder, nil
				default:
					return f.lookup(id)
				}
			}
			inventoryCalls := 0
			dependencies, err := reader.Open(lookup, f.pin(nil)).FreezeDependencies(func() []kernel.RepositoryID {
				inventoryCalls++
				return f.registry.IDs()
			})
			wantDependencies := map[kernel.RepositoryID]kernel.CommitID{f.ops: f.opsAt, f.table: f.tableAt, absent.ID(): absentAt}
			if err != nil || !reflect.DeepEqual(dependencies, wantDependencies) || advanced == "" || advanced == f.opsAt {
				t.Fatalf("preparation mixed descriptor versions: dependencies=%v advanced=%s error=%v", dependencies, advanced, err)
			}
			if moving.headCalls != 1 || nonholder.headCalls != 1 || inventoryCalls != 1 {
				t.Fatalf("candidate versions were reselected: holder=%d nonholder=%d inventory=%d", moving.headCalls, nonholder.headCalls, inventoryCalls)
			}
			for _, commit := range moving.readCommits {
				if commit != f.opsAt {
					t.Fatalf("reference checked outside the frozen dependency: got=%s want=%s", commit, f.opsAt)
				}
			}

			// Interpret both declarations through the actual frozen dependency,
			// after HEAD has advanced. This never invokes an external source.
			scope := reader.Open(lookup, f.pin(dependencies))
			for aspect, origin := range map[string]string{"live-stats": "https://live-origin.example", "live-health": secondOrigin} {
				binding, err := liveaccess.ResolveBindingAt(scope, f.table, objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/shop.orders", AspectName: aspect})
				if err != nil || binding.DescriptorCommit != f.opsAt || binding.Origin != origin {
					t.Fatalf("accepted dependency cannot interpret %s: binding=%#v error=%v", aspect, binding, err)
				}
			}
		})
	}
}

type descriptorDependencyMovingRepository struct {
	objectmodel.Repository
	trigger     objectmodel.ObjectID
	afterRead   func()
	headCalls   int
	readCommits []kernel.CommitID
}

func (r *descriptorDependencyMovingRepository) Head(ref string) (kernel.CommitID, error) {
	r.headCalls++
	return r.Repository.Head(ref)
}

func (r *descriptorDependencyMovingRepository) Read(id objectmodel.ObjectID, commit kernel.CommitID) (objectmodel.ObjectValue, error) {
	r.readCommits = append(r.readCommits, commit)
	value, err := r.Repository.Read(id, commit)
	if err == nil && id == r.trigger && r.afterRead != nil {
		advance := r.afterRead
		r.afterRead = nil
		advance()
	}
	return value, err
}
