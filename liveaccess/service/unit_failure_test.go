package service_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

type unitLookup struct {
	requests []liveaccess.StateLookupRequest
	failure  string
}

func (s *unitLookup) LookupState(_ context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	s.requests = append(s.requests, request)
	observation := liveaccess.StateObservation{Value: map[string]any{"value": float64(31)}, Basis: objectmodel.ObservationBasis{
		BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable,
		SourceRevision: "source-31", ObservedAt: "2026-10-14T00:00:00Z",
	}}
	if request.Binding.Address.Kind == objectmodel.KindMember {
		observation.Value = map[string]any{"value": float64(47)}
	}
	if request.Binding.Address.AspectName == "empty" || request.Binding.Address.MemberKey == "empty" {
		observation.Value = nil
	}
	if request.Binding.Address.AspectName == "broken" || request.Binding.Address.MemberKey == "broken" {
		switch s.failure {
		case "source":
			return liveaccess.StateObservation{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "source unavailable")
		case "basis":
			observation.Value, observation.Basis = nil, objectmodel.ObservationBasis{}
		case "schema":
			observation.Value = map[string]any{"value": "wrong type"}
		}
	}
	return observation, nil
}

const widgetDescriptor objectmodel.ObjectID = "resource/widget-source"

func mixedUnitFixture(t *testing.T, readWrappers ...func(snapshot.Store) snapshot.Store) (*reader.Serving, objectmodel.Repository, kernel.CommitID, objectmodel.ObjectID) {
	t.Helper()
	id := objectmodel.ObjectID("widget/mixed")
	operations := []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{
		Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "definition",
	}, PathHint: "poison/sibling.yaml", Value: map[string]any{"owner": "fixed"}},
		bindingfixture.PutDescriptor(widgetDescriptor, bindingfixture.Descriptor("https://source.example"))}
	for _, aspect := range []string{"good", "empty", "broken"} {
		schemaRef := "schema/widget." + aspect
		operations = append(operations, objectmodel.Operation{Op: objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(schemaRef)},
			Value: map[string]any{"entity": "Widget", "aspect": aspect, "pattern": "record",
				"fields": map[string]any{"value": map[string]any{"type": "number"}}},
		}, bindingfixture.PutAspect(id, aspect, schemaRef, widgetDescriptor))
	}
	return publishUnitFixture(t, id, operations, readWrappers...)
}

func publishUnitFixture(t *testing.T, id objectmodel.ObjectID, operations []objectmodel.Operation, readWrappers ...func(snapshot.Store) snapshot.Store) (*reader.Serving, objectmodel.Repository, kernel.CommitID, objectmodel.ObjectID) {
	t.Helper()
	raw := testkit.MakeTreeStore(t, "kr://liveaccess/mixed-units")
	registry := snapshot.NewRegistry()
	if err := registry.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	receipt, err := w.Commit("publish-mixed-units", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root,
		ExpectedTargetCommit: root, Operations: operations,
	})
	if err != nil {
		t.Fatal(err)
	}
	commit := receipt.Result.NewCommit
	if len(readWrappers) > 0 {
		for _, wrap := range readWrappers {
			raw = wrap(raw)
		}
		registry = snapshot.NewRegistry()
		if err := registry.Add(raw); err != nil {
			t.Fatal(err)
		}
	}
	rd := reader.NewReader(registry)
	repo, err := rd.Require(raw.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	commits := map[kernel.RepositoryID]kernel.CommitID{raw.ID(): commit}
	base := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }, reader.DatasetPin{
		SetID: "mixed", Repositories: commits, Items: reader.WholeRepositoryItems(commits),
	})
	return base, repo, commit, id
}

func assertUnitFailure(t *testing.T, value objectmodel.ObservedValue, address objectmodel.Address, code kernel.ErrorCode) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Failures []struct {
			Address objectmodel.Address `json:"address"`
			Code    kernel.ErrorCode    `json:"code"`
			Message string              `json:"message"`
		} `json:"failures"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	for _, failure := range wire.Failures {
		if failure.Address == address && failure.Code == code && failure.Message != "" {
			for _, observation := range value.Observations {
				if observation.Address == address {
					t.Fatalf("failed unit acquired a successful observation: %#v", value)
				}
			}
			return
		}
	}
	t.Fatalf("unit failure lost its address, code, or reason: %s", raw)
}

func assertFailedUnitRead(t *testing.T, values []objectmodel.ObservedValue, err error, address objectmodel.Address, code kernel.ErrorCode) {
	t.Helper()
	if err != nil || len(values) != 1 || len(values[0].Observations) != 0 || values[0].Digest != "" {
		t.Fatalf("failed external unit did not remain an unobserved local explanation: %#v %v", values, err)
	}
	assertUnitFailure(t, values[0], address, code)
}

func failureCode(kind string) kernel.ErrorCode {
	switch kind {
	case "basis":
		return kernel.ErrCapabilityUnsatisfied
	case "schema":
		return kernel.ErrSchemaInstanceInvalid
	default:
		return kernel.ErrTemporaryUnavailable
	}
}

func TestMixedUnitReadPreservesSnapshotSuccessNullAndLocalFailure(t *testing.T) {
	for _, kind := range []string{"source", "basis", "schema"} {
		t.Run(kind, func(t *testing.T) {
			base, repo, commit, id := mixedUnitFixture(t)
			lookup := &unitLookup{failure: kind}
			service := liveservice.Open(base, lookup, kernel.IdentityContext{Principal: "consumer"})
			values, err := service.Read(context.Background(), id, nil)
			if err != nil || len(values) != 1 {
				t.Fatalf("one failed external unit erased the mixed object: %#v %v", values, err)
			}
			result := values[0]
			body := result.Value.(map[string]any)
			if !reflect.DeepEqual(body["definition"], map[string]any{"owner": "fixed"}) ||
				!reflect.DeepEqual(body["good"], map[string]any{"value": float64(31)}) || body["empty"] != nil {
				t.Fatalf("partial failure changed a fixed, successful, or null value: %#v", body)
			}
			for _, address := range []objectmodel.Address{
				{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "broken"},
			} {
				assertUnitFailure(t, result, address, failureCode(kind))
			}
			if len(result.Observations) != 2 || result.Commit != commit || result.Digest != "" || len(lookup.requests) != 3 {
				t.Fatalf("read lost successful bases, fixed commit, or lookups: %#v %#v", result, lookup.requests)
			}
			fixed, err := repo.Read(id, commit)
			if err != nil || !reflect.DeepEqual(fixed.Value, map[string]any{"definition": map[string]any{"owner": "fixed"}, "good": nil, "empty": nil, "broken": nil}) {
				t.Fatalf("logical hydration changed the fixed declaration: %#v %v", fixed, err)
			}
		})
	}
}

func TestUnitFailureSurvivesAddressAndDeepReferenceSelection(t *testing.T) {
	base, repo, _, id := mixedUnitFixture(t)
	service := liveservice.Open(base, &unitLookup{failure: "source"}, kernel.IdentityContext{Principal: "consumer"})
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "broken"}
	for _, read := range []func() ([]objectmodel.ObservedValue, error){
		func() ([]objectmodel.ObservedValue, error) { return service.ReadAddress(context.Background(), address) },
		func() ([]objectmodel.ObservedValue, error) {
			return service.ReadReference(context.Background(), objectmodel.Reference{Repository: repo.ID(), Object: id, Locator: "/broken/value"})
		},
	} {
		values, err := read()
		if err != nil || len(values) != 1 {
			t.Fatalf("selected external failure disappeared behind a pointer error: %#v %v", values, err)
		}
		assertUnitFailure(t, values[0], address, kernel.ErrTemporaryUnavailable)
		raw, err := json.Marshal(values[0].Value)
		var fault struct {
			Error kernel.IngressError `json:"error"`
		}
		if err != nil || json.Unmarshal(raw, &fault) != nil || fault.Error.Code != kernel.ErrTemporaryUnavailable || fault.Error.Message != "source unavailable" {
			t.Fatalf("failure position did not present its actual reason: %s %v", raw, err)
		}
	}
}

func TestProjectionHydrationRejectsEveryFailedDynamicUnit(t *testing.T) {
	for _, kind := range []string{"source", "basis", "schema"} {
		t.Run(kind, func(t *testing.T) {
			_, repo, commit, id := mixedUnitFixture(t)
			value, err := repo.Read(id, commit)
			if err != nil {
				t.Fatal(err)
			}
			result, err := liveservice.HydrateRepositoryValue(context.Background(), repo, value, &unitLookup{failure: kind}, objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "maintenance"}}, liveaccess.RepositoryDescriptors(repo, commit))
			if kernel.CodeOf(err) != failureCode(kind) || result.Value != nil || len(result.Observations) != 0 {
				t.Fatalf("projection accepted a failed dynamic value: %#v %v", result, err)
			}
		})
	}
}
