package invocation

import (
	"encoding/json"
	"reflect"
	"testing"

	retrieval "kc/discovery"
	kchome "kc/home"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func newComposedMemberDeliveryFixture(t *testing.T) namedConsumptionFixture {
	t.Helper()
	f := newNamedConsumptionFixture(t)
	repo := f.repos[1]
	head := f.pin.Repositories[repo.ID()]
	if _, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "metric/gmv", AspectName: "quality"},
			Value:   map[string]any{"status": "current"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ws.Datasets.DefineDataset("analysis", 2, f.sources); err != nil {
		t.Fatal(err)
	}
	var err error
	f.pin, err = f.ws.Datasets.ResolveDataset("analysis")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(f.rules); i++ {
		f.rules[i].Ref = string(f.pin.Repositories[kernel.RepositoryID(f.rules[i].Repo)])
	}
	f.allow(t, f.rules)
	return f
}

func composedMemberDeliveryFlags(f namedConsumptionFixture) map[string]invoke.FlagValue {
	flags := f.flags()
	flags["revision"] = "2"
	return flags
}

func composedMemberDeliveryHit(t *testing.T, f namedConsumptionFixture, serving *reader.Serving) retrieval.ObjectHit {
	t.Helper()
	values := []objectmodel.ObjectValue{}
	for _, repo := range f.repos {
		member, err := serving.Member(repo.ID())
		if err != nil {
			t.Fatal(err)
		}
		value, err := member.Read("metric/gmv", serving.Pin().Repositories[repo.ID()])
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, value)
	}
	value, err := objectmodel.ComposeHoldingValues("urn:kc:object:metric/gmv", values)
	if err != nil {
		t.Fatal(err)
	}
	hit := retrieval.ObjectHit{Object: value, Version: retrieval.VersionOf(value)}
	// Delivery preserves adopted observation metadata; LiveAccess owns its
	// original adoption and evidence validation, independently of this stage.
	hit.Version.Observations = []objectmodel.UnitObservation{{
		Address:           objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "metric/gmv", AspectName: "quality"},
		DeclarationCommit: values[1].Commit, DeclarationDigest: kernel.CanonicalDigest("quality-declaration"),
		Basis: objectmodel.ObservationBasis{BindingGeneration: "quality-runtime", Consistency: objectmodel.ObservationRepeatable,
			SourceRevision: "quality-7", ObservedAt: "2026-10-16T00:00:00Z"},
	}}
	return hit
}

func TestComposedMemberDeliveryKeepsTypedBodyAndObservations(t *testing.T) {
	for _, grant := range []string{"complete members", "Dataset"} {
		t.Run(grant, func(t *testing.T) {
			f := newComposedMemberDeliveryFixture(t)
			if grant == "Dataset" {
				f.allow(t, []AllowRule{f.rules[0], {Principal: "reader", Dataset: "analysis", Actions: []string{"file.read"}}})
			}
			called := false
			result := f.invoke("file.read", composedMemberDeliveryFlags(f), func(cx *Invocation) (any, error) {
				called = true
				serving, _, err := OpenCompleteServing(cx, "metric/gmv")
				if err != nil {
					return nil, err
				}
				hit := composedMemberDeliveryHit(t, f, serving)
				delivered, err := DeliverSearchHit(cx.Home, cx.Flags, hit)
				if err != nil || !reflect.DeepEqual(delivered, hit) || delivered.Object.Repository != "" || len(delivered.Object.Sources) != 2 {
					t.Fatalf("authorized composed delivery lost body, observations or identity: got=%#v want=%#v error=%v", delivered, hit, err)
				}
				return namedConsumptionSearchOutput(serving.Pin(), delivered), nil
			})
			if result.Status != 0 || !called {
				t.Fatalf("typed composed consumption failed: called=%v result=%#v", called, result)
			}
		})
	}
}

func TestComposedMemberDeliveryRechecksRevocation(t *testing.T) {
	for _, revoke := range []string{"member", "resolve"} {
		t.Run(revoke, func(t *testing.T) {
			f := newComposedMemberDeliveryFixture(t)
			flags := composedMemberDeliveryFlags(f)
			result := f.invoke("file.read", flags, func(cx *Invocation) (any, error) {
				serving, _, err := OpenCompleteServing(cx, "metric/gmv")
				if err != nil {
					return nil, err
				}
				hit := composedMemberDeliveryHit(t, f, serving)
				first, err := DeliverSearchHit(cx.Home, cx.Flags, hit)
				if err != nil || first.Object.Value == nil || len(first.Version.Observations) != 1 {
					t.Fatalf("initial composed delivery failed: %#v error=%v", first, err)
				}
				rules := f.rules[:2]
				if revoke == "resolve" {
					rules = f.rules[1:]
				}
				f.allow(t, rules)
				delivered := assertComposedMemberDeliveryStripped(t, cx.Home, cx.Flags, hit)
				return namedConsumptionSearchOutput(serving.Pin(), delivered), nil
			})
			if result.Status != 0 {
				t.Fatalf("in-flight revocation probe failed: %#v", result)
			}
			called := false
			next := f.invoke("file.read", flags, func(*Invocation) (any, error) { called = true; return nil, nil })
			if next.Status == 0 || called || namedConsumptionError(t, next) != kernel.ErrForbidden {
				t.Fatalf("next invocation reused revoked consumption: called=%v result=%#v", called, next)
			}
		})
	}
}

func TestComposedMemberDeliveryRejectsUntrustedSources(t *testing.T) {
	for _, scenario := range []string{"no sources", "missing source", "unknown source", "empty source", "empty commit", "wrong commit", "duplicate source", "missing aspects", "unknown aspect", "duplicate aspect", "unattributed value", "wrong object", "single commit", "single reference repository"} {
		t.Run(scenario, func(t *testing.T) {
			f := newComposedMemberDeliveryFixture(t)
			result := f.invoke("file.read", composedMemberDeliveryFlags(f), func(cx *Invocation) (any, error) {
				serving, _, err := OpenCompleteServing(cx, "metric/gmv")
				if err != nil {
					return nil, err
				}
				hit := composedMemberDeliveryHit(t, f, serving)
				switch scenario {
				case "no sources":
					hit.Object.Sources = nil
				case "missing source":
					hit.Object.Sources = hit.Object.Sources[:1]
				case "unknown source":
					hit.Object.Sources[1].Repository = "kr://consumption/outside"
				case "empty source":
					hit.Object.Sources[1].Repository = ""
				case "empty commit":
					hit.Object.Sources[1].Commit = ""
				case "wrong commit":
					hit.Object.Sources[1].Commit = "not-the-accepted-commit"
				case "duplicate source":
					hit.Object.Sources[1] = hit.Object.Sources[0]
				case "missing aspects":
					hit.Object.Sources[1].Aspects = nil
				case "unknown aspect":
					hit.Object.Sources[1].Aspects = []string{"outside"}
				case "duplicate aspect":
					hit.Object.Sources[1].Aspects = hit.Object.Sources[0].Aspects
				case "unattributed value":
					hit.Object.Value.(map[string]any)["outside"] = "not covered by a source"
				case "wrong object":
					hit.Object.ObjectRef.Object = "metric/other"
				case "single commit":
					hit.Object.Commit = f.pin.Repositories[f.repos[0].ID()]
				case "single reference repository":
					hit.Object.ObjectRef.Repository = f.repos[0].ID()
				}
				assertComposedMemberDeliveryStripped(t, cx.Home, cx.Flags, hit)
				// This probe injected an invalid internal candidate. The delivery
				// assertion above is independent of the recorder's strict basis check.
				return namedConsumptionSearchOutput(serving.Pin()), nil
			})
			if result.Status != 0 {
				t.Fatalf("source validation probe failed: %#v", result)
			}
		})
	}
}

func TestComposedMemberDeliveryRequiresAcceptedNamedScope(t *testing.T) {
	f := newComposedMemberDeliveryFixture(t)
	flags := composedMemberDeliveryFlags(f)
	// Wire coordinates and a plausible source vector alone cannot create the
	// private accepted scope that the typed authorization boundary supplies.
	serving := reader.Open(f.ws.Reader.Lookup(f.ws.Datasets.Require), kchome.DatasetPin(f.pin))
	hit := composedMemberDeliveryHit(t, f, serving)
	assertComposedMemberDeliveryStripped(t, f.ws.Dir, flags, hit)
	delete(flags, "dataset")
	delete(flags, "revision")
	flags["repo"], flags["ref"] = string(f.repos[0].ID()), string(f.pin.Repositories[f.repos[0].ID()])
	called := false
	denied := f.invoke("file.read", flags, func(*Invocation) (any, error) { called = true; return nil, nil })
	if denied.Status == 0 || called || namedConsumptionError(t, denied) != kernel.ErrForbidden {
		t.Fatalf("member grants authorized direct Repository consumption: called=%v result=%#v", called, denied)
	}
	assertComposedMemberDeliveryStripped(t, f.ws.Dir, flags, hit)
	// An explicit direct Repository grant retains its ordinary single-source
	// delivery behavior, without authorizing the composed empty-Repository hit.
	f.allow(t, append(f.rules, AllowRule{Principal: "reader", Repo: string(f.repos[0].ID()), Actions: []string{"object.read"}}))
	raw, err := f.repos[0].Read("metric/gmv", f.pin.Repositories[f.repos[0].ID()])
	if err != nil {
		t.Fatal(err)
	}
	got, err := DeliverSearchHit(f.ws.Dir, flags, retrieval.ObjectHit{Object: raw})
	if err != nil || !reflect.DeepEqual(got.Object, raw) {
		t.Fatalf("direct Repository policy changed: %#v error=%v", got, err)
	}
	assertComposedMemberDeliveryStripped(t, f.ws.Dir, flags, hit)
}

func TestComposedMemberReadUsesTypedVerbAndFixedSources(t *testing.T) {
	f := newComposedMemberDeliveryFixture(t)
	flags := composedMemberDeliveryFlags(f)
	flags["reference"] = "urn:kc:object:metric/gmv"
	result := f.invoke("object.read", flags, VerbRead)
	if result.Status != 0 {
		t.Fatalf("typed composed READ rejected complete member grants: %#v", result)
	}
	var value objectmodel.ObservedValue
	if err := json.Unmarshal([]byte(result.Stdout), &value); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"definition": map[string]any{"name": "metric/gmv"}, "quality": map[string]any{"status": "current"}}
	if value.Repository != "" || value.ObjectID != "metric/gmv" || !reflect.DeepEqual(value.Value, want) || len(value.Sources) != 2 {
		t.Fatalf("typed READ lost the fixed composed object: %#v", value)
	}
	for _, source := range value.Sources {
		if source.Commit == "" || source.Commit != f.pin.Repositories[source.Repository] {
			t.Fatalf("typed READ escaped the accepted source versions: %#v", value)
		}
	}
}

func TestComposedMemberReadFiltersRevokedOrForgedSources(t *testing.T) {
	for _, scenario := range []string{"member", "resolve", "missing source", "unknown source", "wrong commit", "single reference repository"} {
		t.Run(scenario, func(t *testing.T) {
			f := newComposedMemberDeliveryFixture(t)
			flags := composedMemberDeliveryFlags(f)
			flags["reference"] = "urn:kc:object:metric/gmv"
			result := f.invoke("object.read", flags, func(cx *Invocation) (any, error) {
				raw, err := VerbRead(cx)
				if err != nil {
					return nil, err
				}
				value, ok := raw.(objectmodel.ObservedValue)
				if !ok || value.Value == nil || len(value.Sources) != 2 {
					t.Fatalf("actual composed READ did not reach the source filter: %#v", raw)
				}
				if len(FilterObjectServingReads(cx.Home, cx.Flags, nil, []objectmodel.ObservedValue{value})) != 1 {
					t.Fatal("valid composed READ failed repeated source filtering")
				}
				switch scenario {
				case "member":
					f.allow(t, f.rules[:2])
				case "resolve":
					f.allow(t, f.rules[1:])
				case "missing source":
					value.Sources = value.Sources[:1]
				case "unknown source":
					value.Sources[1].Repository = "kr://consumption/outside"
				case "wrong commit":
					value.Sources[1].Commit = "not-the-accepted-commit"
				case "single reference repository":
					value.ObjectRef.Repository = f.repos[0].ID()
				}
				if filtered := FilterObjectServingReads(cx.Home, cx.Flags, nil, []objectmodel.ObservedValue{value}); len(filtered) != 0 {
					t.Fatalf("READ source filter retained revoked or forged content: %#v", filtered)
				}
				return "source filter rechecked", nil
			})
			if result.Status != 0 {
				t.Fatalf("typed READ source filter probe failed: %#v", result)
			}
		})
	}
}

func assertComposedMemberDeliveryStripped(t *testing.T, home string, flags map[string]invoke.FlagValue, hit retrieval.ObjectHit) retrieval.ObjectHit {
	t.Helper()
	delivered, err := DeliverSearchHit(home, flags, hit)
	if err != nil || delivered.Object.Value != nil || len(delivered.Version.Observations) != 0 || len(delivered.Object.Declarations) != 0 || len(delivered.Object.Units) != 0 ||
		!reflect.DeepEqual(delivered.Object.Sources, hit.Object.Sources) || delivered.Object.ObjectRef != hit.Object.ObjectRef {
		t.Fatalf("invalid composed delivery retained content or changed identity: %#v error=%v", delivered, err)
	}
	return delivered
}
