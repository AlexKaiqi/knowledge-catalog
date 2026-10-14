package service_test

import (
	"context"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/liveaccess/evidence"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func memberUnitFixture(t *testing.T, readWrappers ...func(snapshot.Store) snapshot.Store) (*reader.Serving, objectmodel.Repository, kernel.CommitID, objectmodel.ObjectID) {
	t.Helper()
	id := objectmodel.ObjectID("widget/members")
	operations := []objectmodel.Operation{{Op: objectmodel.OpPut,
		Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/widget.readings"},
		Value: map[string]any{"entity": "Widget", "aspect": "readings", "pattern": "keyed_collection", "origin": "https://source.example",
			"fields": map[string]any{"value": map[string]any{"type": "number", "required": true, "access": []any{"filter"}},
				"fresh": map[string]any{"type": "number"}}},
	}}
	for _, key := range []string{"good", "empty", "broken", "fixed"} {
		op := objectmodel.Operation{Op: objectmodel.OpPut,
			Address:   objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: id, AspectName: "readings", MemberKey: key},
			SchemaRef: "schema/widget.readings", Value: map[string]any{"value": float64(31)},
		}
		if key == "fixed" {
			op.SchemaRef, op.Value = "", map[string]any{"owner": "fixed"}
			op.PathHint = "poison/sibling.yaml"
		}
		operations = append(operations, op)
	}
	return publishUnitFixture(t, id, operations, readWrappers...)
}

type memberSchemaReadGuard struct {
	objectmodel.Repository
	schemaReads int
}

func (r *memberSchemaReadGuard) Read(id objectmodel.ObjectID, at kernel.CommitID) (objectmodel.ObjectValue, error) {
	if objectmodel.IsSchemaObject(id) {
		r.schemaReads++
	}
	return r.Repository.Read(id, at)
}

func (r *memberSchemaReadGuard) ObjectUnitPaths(id objectmodel.ObjectID, at kernel.CommitID) ([]string, error) {
	return r.Repository.(objectmodel.UnitLocator).ObjectUnitPaths(id, at)
}

func TestDeclaredMemberSchemaFailureStaysLocalWithoutPrivateRead(t *testing.T) {
	_, repo, commit, id := memberUnitFixture(t)
	guard := &memberSchemaReadGuard{Repository: repo}
	paths, err := guard.ObjectUnitPaths(id, commit)
	if err != nil {
		t.Fatal(err)
	}
	items := make([]reader.DatasetItem, 0, len(paths))
	for _, path := range paths {
		items = append(items, reader.DatasetItem{Repository: repo.ID(), Commit: commit, Kind: "file", File: path})
	}
	scope := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return guard, nil }, reader.DatasetPin{
		Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit}, Items: items,
	})
	lookup := &unitLookup{failure: "source"}
	collector := evidence.NewCollector()
	ctx := evidence.WithCollector(context.Background(), collector)
	values, err := liveservice.Open(scope, lookup, kernel.IdentityContext{Principal: "consumer"}).Read(ctx, id, nil)
	if err != nil || len(values) != 1 || len(values[0].Observations) != 0 || len(lookup.requests) != 0 || guard.schemaReads != 0 {
		t.Fatalf("Member Schema failure erased siblings or crossed publication scope: %#v calls=%d private=%d %v", values, len(lookup.requests), guard.schemaReads, err)
	}
	for _, key := range []string{"good", "empty", "broken"} {
		assertUnitFailure(t, values[0], objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: id, AspectName: "readings", MemberKey: key}, kernel.ErrObjectRefUnresolved)
	}
	if !reflect.DeepEqual(values[0].Value.(map[string]any)["readings"].(map[string]any)["fixed"], map[string]any{"owner": "fixed"}) {
		t.Fatalf("unavailable Schema changed the static Member: %#v", values)
	}
	samples := collectedSamples(t, collector)
	if len(samples) != 3 {
		t.Fatalf("unavailable declared handles lost their retained failures: %#v", samples)
	}
	for _, sample := range samples {
		if sample.Outcome != evidence.FailureOutcome || sample.Declaration.SchemaRef != "schema/widget.readings" || sample.Observation != nil {
			t.Fatalf("failed Schema did not retain its original declaration: %#v", sample)
		}
	}
}

func TestMemberReadAdoptsExternalValueAndKeepsIndependentFailures(t *testing.T) {
	for _, kind := range []string{"source", "basis", "schema"} {
		t.Run(kind, func(t *testing.T) {
			base, repo, commit, id := memberUnitFixture(t)
			lookup := &unitLookup{failure: kind}
			service := liveservice.Open(base, lookup, kernel.IdentityContext{Principal: "consumer"})
			values, err := service.Read(context.Background(), id, nil)
			if err != nil || len(values) != 1 {
				t.Fatalf("one Member failure erased its siblings: %#v %v", values, err)
			}
			result := values[0]
			members := result.Value.(map[string]any)["readings"].(map[string]any)
			if !reflect.DeepEqual(members["good"], map[string]any{"value": float64(47)}) || members["empty"] != nil ||
				!reflect.DeepEqual(members["fixed"], map[string]any{"owner": "fixed"}) {
				t.Fatalf("Member read used the stored placeholder or lost a sibling: %#v", members)
			}
			assertUnitFailure(t, result, objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: id, AspectName: "readings", MemberKey: "broken"}, failureCode(kind))
			if len(result.Observations) != 2 || len(lookup.requests) != 3 || result.Commit != commit {
				t.Fatalf("Member read lost its independently validated observations: %#v %#v", result, lookup.requests)
			}
			fixed, err := repo.ReadAddress(objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: id, AspectName: "readings", MemberKey: "good"}, commit)
			if err != nil || !reflect.DeepEqual(fixed.Value, map[string]any{"value": float64(31)}) {
				t.Fatalf("external value changed its fixed Member: %#v %v", fixed, err)
			}
		})
	}
}

func TestRepositoryReadHydratesOnlySelectedMemberShape(t *testing.T) {
	_, repo, commit, id := memberUnitFixture(t)
	address := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: id, AspectName: "readings", MemberKey: "good"}
	port := repo.(objectmodel.ReferenceReader)
	request := objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "consumer"}}
	for _, tc := range []struct {
		name string
		read func() (objectmodel.ObjectValue, error)
		want any
	}{
		{"address", func() (objectmodel.ObjectValue, error) { return repo.ReadAddress(address, commit) }, map[string]any{"value": float64(47)}},
		{"leaf", func() (objectmodel.ObjectValue, error) {
			return port.ReadReference(objectmodel.Reference{Repository: repo.ID(), Object: id, Locator: "/readings/good/value"}, commit)
		}, float64(47)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := tc.read()
			if err != nil {
				t.Fatal(err)
			}
			lookup := &unitLookup{failure: "source"}
			result, err := liveservice.HydrateRepositoryRead(context.Background(), repo, raw, lookup, request, &objectmodel.AspectSelector{Include: []string{"readings"}})
			if err != nil || !reflect.DeepEqual(result.Value, tc.want) || result.Address != address || result.Commit != commit ||
				result.Reference != raw.Reference || result.Locator != raw.Locator || len(result.Observations) != 1 || len(result.Failures) != 0 || len(lookup.requests) != 1 {
				t.Fatalf("selected Member read changed shape, basis, or visited siblings: %#v %#v %v", result, lookup.requests, err)
			}
		})
	}
	static, err := repo.ReadAddress(objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: id, AspectName: "readings", MemberKey: "fixed"}, commit)
	if err != nil {
		t.Fatal(err)
	}
	result, err := liveservice.HydrateRepositoryRead(context.Background(), repo, static, nil, request, nil)
	if err != nil || !reflect.DeepEqual(result.Value, static.Value) || result.Digest != static.Digest || len(result.Observations) != 0 || len(result.Failures) != 0 {
		t.Fatalf("static selected Member changed: %#v %v", result, err)
	}
}

func TestRepositoryReadKeepsSelectedMemberCollectionFailurePositions(t *testing.T) {
	_, repo, commit, id := memberUnitFixture(t)
	raw, err := repo.(objectmodel.ReferenceReader).ReadReference(objectmodel.Reference{Repository: repo.ID(), Object: id, Locator: "/readings"}, commit)
	if err != nil {
		t.Fatal(err)
	}
	lookup := &unitLookup{failure: "source"}
	result, err := liveservice.HydrateRepositoryRead(context.Background(), repo, raw, lookup,
		objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "consumer"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	members := result.Value.(map[string]any)
	if !reflect.DeepEqual(members["good"], map[string]any{"value": float64(47)}) || members["empty"] != nil ||
		!reflect.DeepEqual(members["fixed"], map[string]any{"owner": "fixed"}) || len(lookup.requests) != 3 || len(result.Observations) != 2 || result.Locator != "/readings" {
		t.Fatalf("selected collection lost unit positions or visited unrelated content: %#v %#v", result, lookup.requests)
	}
	assertUnitFailure(t, result, objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: id, AspectName: "readings", MemberKey: "broken"}, kernel.ErrTemporaryUnavailable)
	if _, ok := members["broken"].(liveservice.ValueFailure); !ok {
		t.Fatalf("Member failure was not explained at its key: %#v", members)
	}
	if !reflect.DeepEqual(raw.Value.(map[string]any)["good"], map[string]any{"value": float64(31)}) ||
		!reflect.DeepEqual(raw.Value.(map[string]any)["broken"], map[string]any{"value": float64(31)}) {
		t.Fatalf("consumer hydration mutated the fixed selected payload: %#v", raw)
	}
}

func TestMemberProjectionHydrationRejectsFailure(t *testing.T) {
	_, repo, commit, id := memberUnitFixture(t)
	raw, err := repo.Read(id, commit)
	if err != nil {
		t.Fatal(err)
	}
	result, err := liveservice.HydrateRepositoryValue(context.Background(), repo, raw, &unitLookup{failure: "source"}, objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "maintenance"}})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || result.Value != nil || len(result.Observations) != 0 {
		t.Fatalf("index hydration admitted a failed Member explanation: %#v %v", result, err)
	}
}
