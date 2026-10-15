package service_test

import (
	"context"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

type historicalSchemaReadGuard struct {
	*repositorySiblingGuard
	schemaReads map[kernel.CommitID]int
}

func (g *historicalSchemaReadGuard) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	if path == "schemas/type.yaml" {
		g.schemaReads[at]++
	}
	return g.repositorySiblingGuard.ReadFile(path, at)
}

func TestBindingAspectUsesHistoricalAdoptedSchemaWithoutSiblingReads(t *testing.T) {
	raw := testkit.MakeTreeStore(t, "kr://liveaccess/historical-adoption")
	registry := snapshot.NewRegistry()
	if err := registry.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	commit := testkit.MustHead(t, raw, snapshot.DefaultRef)
	put := func(command string, operations []objectmodel.Operation) kernel.CommitID {
		t.Helper()
		receipt, err := w.Commit(command, objectmodel.ChangeSet{TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
			BaseCommit: commit, ExpectedTargetCommit: commit, Operations: operations})
		if err != nil {
			t.Fatal(err)
		}
		commit = receipt.Result.NewCommit
		return commit
	}
	metric := func(fields map[string]any) map[string]any {
		fields["value"] = map[string]any{"type": "number"}
		return map[string]any{"entity": "Widget", "aspect": "metric", "fields": fields}
	}
	schemaBasis := put("publish-typed-schemas", []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/widget.metric"}, PathHint: "schemas/type.yaml", Value: metric(map[string]any{})},
		bindingfixture.PutDescriptor("resource/widget-metric", bindingfixture.Descriptor("https://widget.example")),
	})
	id := objectmodel.ObjectID("thing/one")
	ref := "schema/widget.metric@" + string(schemaBasis)
	put("publish-historically-typed-object", []objectmodel.Operation{{Op: objectmodel.OpPut,
		Address:  objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "definition"},
		PathHint: "poison/sibling.yaml", Value: map[string]any{"owner": "fixed"},
	}, bindingfixture.PutAspect(id, "metric", ref, "resource/widget-metric")})
	put("advance-current-schema-body", []objectmodel.Operation{{Op: objectmodel.OpPut,
		Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/widget.metric"}, Value: metric(map[string]any{"later": map[string]any{"type": "number"}}),
	}})
	guard := &historicalSchemaReadGuard{repositorySiblingGuard: &repositorySiblingGuard{Store: raw, TreeStore: raw.(snapshot.TreeStore)}, schemaReads: map[kernel.CommitID]int{}}
	readRegistry := snapshot.NewRegistry()
	if err := readRegistry.Add(guard); err != nil {
		t.Fatal(err)
	}
	repo, err := reader.NewReader(readRegistry).Require(raw.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := repo.(objectmodel.SchemaAdoptionLocator).AdoptedSchemaRefs(id, commit)
	if err != nil || len(refs) != 1 || refs[0] != ref || guard.siblingReads != 0 {
		t.Fatalf("fixed adoption metadata lost its historical pin or read a sibling: %v sibling=%d %v", refs, guard.siblingReads, err)
	}
	lookup := &unitLookup{}
	result, err := liveservice.ReadRepository(context.Background(), repo, liveservice.ReadRequest{Commit: commit,
		Reference: &objectmodel.Reference{Repository: repo.ID(), Object: id, Locator: "/metric/value"}}, lookup,
		objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "consumer"}})
	if err != nil || result.Value != float64(31) || len(lookup.requests) != 1 || lookup.requests[0].Origin != "https://widget.example" || guard.siblingReads != 0 ||
		guard.schemaReads[schemaBasis] == 0 || guard.schemaReads[commit] != 0 {
		t.Fatalf("binding Aspect followed current Schema instead of the actual historical adoption: %#v %#v sibling=%d schema=%v %v", result, lookup.requests, guard.siblingReads, guard.schemaReads, err)
	}
	commits := map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit}
	scope := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }, reader.DatasetPin{
		Repositories: commits, Items: reader.WholeRepositoryItems(commits),
	})
	member, err := scope.Member(repo.ID())
	if err != nil {
		t.Fatal(err)
	}
	oldReads := guard.schemaReads[schemaBasis]
	rejected, err := liveservice.ReadRepository(context.Background(), member, liveservice.ReadRequest{Commit: commit,
		Reference: &objectmodel.Reference{Repository: repo.ID(), Object: id, Locator: "/metric/value"}}, lookup,
		objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "consumer"}})
	failure, failed := rejected.Value.(liveservice.ValueFailure)
	if err != nil || !failed || failure.Error.Code != kernel.ErrObjectRefUnresolved || len(rejected.Observations) != 0 || len(lookup.requests) != 1 ||
		guard.schemaReads[schemaBasis] != oldReads || guard.siblingReads != 0 {
		t.Fatalf("Repository interpretation bypassed its supplied Dataset version boundary: %#v lookups=%d sibling=%d schema=%v %v", rejected, len(lookup.requests), guard.siblingReads, guard.schemaReads, err)
	}
}
