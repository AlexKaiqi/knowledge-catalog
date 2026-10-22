package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"kc/kernel"
	"kc/liveaccess"
	"kc/liveaccess/evidence"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
)

type lookupFunc func(context.Context, liveaccess.StateLookupRequest) (liveaccess.StateObservation, error)

func (f lookupFunc) LookupState(ctx context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	return f(ctx, request)
}

func collectedSamples(t *testing.T, collector *evidence.Collector) []evidence.Sample {
	t.Helper()
	samples, err := collector.Values()
	if err != nil {
		t.Fatal(err)
	}
	return samples
}

func TestReadCapturesOnlyAdoptedDynamicUnits(t *testing.T) {
	base, repo, commit, id := mixedUnitFixture(t)
	lookup := &unitLookup{failure: "source"}
	collector := evidence.NewCollector()
	ctx := evidence.WithCollector(context.Background(), collector)
	service := liveservice.Open(base, lookup, kernel.IdentityContext{Principal: "consumer"})
	// Two coordinates are read on their own: the unselected "broken" unit must
	// never be visited, which is what the retired whole-object selector used to
	// express by trimming after the fact.
	var values []objectmodel.ObservedValue
	for _, aspect := range []string{"good", "empty"} {
		read, err := service.ReadAddress(ctx, objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: aspect})
		if err != nil || len(read) != 1 {
			t.Fatalf("coordinate read %s: %#v %v", aspect, read, err)
		}
		values = append(values, read...)
	}
	if len(lookup.requests) != 2 {
		t.Fatalf("coordinate reads visited an unselected unit: %#v", lookup.requests)
	}
	_ = repo
	samples := collectedSamples(t, collector)
	if len(samples) != 2 {
		t.Fatalf("coordinates retained extra or missing units: %#v", samples)
	}
	for _, sample := range samples {
		if sample.Commit != commit || sample.Outcome != evidence.ValueOutcome || sample.Observation == nil || sample.Failure != nil ||
			sample.Declaration.SchemaRef == "" || sample.Declaration.DeclarationDigest != sample.Observation.DeclarationDigest {
			t.Fatalf("adopted value lost its fixed declaration or successful basis: %#v", sample)
		}
		if sample.Address.AspectName == "empty" && sample.Value != nil {
			t.Fatalf("observed null was lost from retained results: %#v", sample)
		}
	}
	values[0].Value.(map[string]any)["value"] = "caller mutation"
	for _, sample := range collectedSamples(t, collector) {
		if sample.Address.AspectName == "good" && kernel.CanonicalDigest(sample.Value) != kernel.CanonicalDigest(map[string]any{"value": float64(31)}) {
			t.Fatalf("caller mutation altered the retained original value: %#v", sample)
		}
	}
}

func TestDeepMemberReadRetainsAdoptedCompleteUnit(t *testing.T) {
	base, repo, commit, id := memberUnitFixture(t)
	collector := evidence.NewCollector()
	ctx := evidence.WithCollector(context.Background(), collector)
	lookup := &unitLookup{failure: "source"}
	values, err := liveservice.Open(base, lookup, kernel.IdentityContext{Principal: "consumer"}).ReadReference(ctx,
		objectmodel.Reference{Repository: repo.ID(), Object: id, Locator: "/readings/good/value"})
	if err != nil || len(values) != 1 || values[0].Value != float64(47) || values[0].Locator != "/readings/good/value" || len(lookup.requests) != 1 {
		t.Fatalf("deep Member read changed selection or consulted siblings: %#v %#v %v", values, lookup.requests, err)
	}
	samples := collectedSamples(t, collector)
	if len(samples) != 1 || samples[0].Commit != commit || samples[0].Address.MemberKey != "good" ||
		kernel.CanonicalDigest(samples[0].Value) != kernel.CanonicalDigest(map[string]any{"value": float64(47)}) ||
		samples[0].Declaration.ValueSource == nil || samples[0].Declaration.ValueSource.Kind != objectmodel.ValueSourceBinding ||
		samples[0].Observation == nil || samples[0].Declaration.DeclarationDigest != samples[0].Observation.DeclarationDigest {
		t.Fatalf("leaf could not be reviewed against the original adopted Member: %#v", samples)
	}
}

func TestInjectedLookupCredentialsCannotReachResultOrEvidence(t *testing.T) {
	for _, kind := range []string{"error", "value", "basis"} {
		t.Run(kind, func(t *testing.T) {
			base, _, _, id := mixedUnitFixture(t)
			collector := evidence.NewCollector()
			ctx := evidence.WithCollector(context.Background(), collector)
			ctx = liveaccess.ContextWithCallerCredentials(ctx, liveaccess.CallerCredentialsFromHeader(http.Header{"Authorization": []string{"Bearer caller-proof"}}))
			lookup := lookupFunc(func(ctx context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
				if kind == "error" {
					return liveaccess.StateObservation{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "source rejected Bearer caller-proof")
				}
				observation, err := (&unitLookup{}).LookupState(ctx, request)
				if kind == "value" {
					observation.Value.(map[string]any)["proof"] = "caller-proof"
				} else {
					observation.Basis.SourceRevision = "caller-proof"
				}
				return observation, err
			})
			values, err := liveservice.Open(base, lookup, kernel.IdentityContext{Principal: "consumer"}).ReadAddress(ctx,
				objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "broken"})
			code := kernel.ErrCapabilityUnsatisfied
			if kind == "error" {
				code = kernel.ErrTemporaryUnavailable
			}
			assertFailedUnitRead(t, values, err, objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "broken"}, code)
			samples := collectedSamples(t, collector)
			if len(samples) != 1 || samples[0].Outcome != evidence.FailureOutcome || samples[0].Value != nil || samples[0].Observation != nil || samples[0].Failure == nil || samples[0].Failure.Code != code {
				t.Fatalf("rejected source result became a successful retained observation: %#v", samples)
			}
			body, err := json.Marshal(struct {
				Values  []objectmodel.ObservedValue
				Samples []evidence.Sample
			}{values, samples})
			if err != nil || strings.Contains(string(body), "caller-proof") {
				t.Fatalf("credential reached the result or retained evidence: %s %v", body, err)
			}
		})
	}
}

func TestCaptureFailurePreventsConsumerDelivery(t *testing.T) {
	base, _, _, id := mixedUnitFixture(t)
	collector := evidence.NewCollector()
	ctx := evidence.WithCollector(context.Background(), collector)
	lookup := lookupFunc(func(ctx context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
		observation, err := (&unitLookup{}).LookupState(ctx, request)
		observation.Value.(map[string]any)["unencodable"] = func() {}
		return observation, err
	})
	values, err := liveservice.Open(base, lookup, kernel.IdentityContext{Principal: "consumer"}).ReadAddress(ctx,
		objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "good"})
	if err == nil || values != nil || len(collectedSamples(t, collector)) != 0 {
		t.Fatalf("failed evidence capture still delivered a consumer result: %#v %v", values, err)
	}
}
