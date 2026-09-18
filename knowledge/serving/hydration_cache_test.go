package serving_test

import (
	"context"
	"reflect"
	"testing"

	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/serving"
	"kc/observability"
	readcache "kc/retrieval/cache"
)

func TestHotSnapshotCacheKeepsStateObservationsFreshAndDeclarationsUnchanged(t *testing.T) {
	for _, shape := range []string{"object"} {
		t.Run(shape, func(t *testing.T) {
			base, repositoryID, commit, address := setupServing(t)
			cache, err := readcache.New(readcache.Config{MaxEntries: 4})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cache.Close() })
			base.SetHydrator(cache)
			readSnapshot := func() ([]reader.FederatedValue, error) {
				return base.Read(address.ObjectID, nil)
			}
			original, err := readSnapshot()
			if err != nil || len(original) != 1 || original[0].Commit != commit {
				t.Fatalf("warm declaration: %+v %v", original, err)
			}
			warm := cache.Stats()
			if warm.Entries != 1 || warm.SourceReads != 1 {
				t.Fatalf("declaration was not retained: %+v", warm)
			}
			identity := observability.IdentityContext{Principal: "agent", OnBehalfOf: "alice"}
			lookup := &stateLookup{}
			logical := serving.Open(base, lookup, identity)
			declarationDigest := ""
			for _, observation := range []struct{ status, revision, observedAt string }{
				{"running", "job-rev-42", "2026-09-08T08:30:00Z"},
				{"completed", "job-rev-43", "2026-09-08T08:31:00Z"},
			} {
				lookup.result = serving.StateObservation{
					Value: map[string]any{"status": observation.status},
					Basis: knowledge.ObservationBasis{
						BindingGeneration: "scheduler-config-7", Consistency: knowledge.ObservationRepeatable,
						SourceRevision: observation.revision, ObservedAt: observation.observedAt,
					},
				}
				var results []serving.ReadResult
				if shape == "address" {
					results, err = logical.ReadAddress(context.Background(), address)
				} else {
					results, err = logical.Read(context.Background(), address.ObjectID, nil)
				}
				if err != nil || len(results) != 1 {
					t.Fatalf("logical read: %+v %v", results, err)
				}
				result := results[0]
				body := result.Value
				if shape == "object" {
					body = result.Value.(map[string]any)[address.AspectName]
				}
				if !reflect.DeepEqual(body, lookup.result.Value) {
					t.Fatalf("hot cache replayed an old State value: got %#v want %#v", body, lookup.result.Value)
				}
				if result.Repository != repositoryID || result.Commit != commit || len(result.Observations) != 1 {
					t.Fatalf("logical read lost its fixed declaration coordinates: %+v", result)
				}
				version := result.Observations[0]
				if version.Address != address || version.DeclarationCommit != commit || version.DeclarationDigest == "" || version.Basis != lookup.result.Basis {
					t.Fatalf("logical read mixed declaration and observation basis: %+v", version)
				}
				if declarationDigest != "" && string(version.DeclarationDigest) != declarationDigest {
					t.Fatal("unchanged Snapshot declaration acquired another digest")
				}
				declarationDigest = string(version.DeclarationDigest)
				raw, err := readSnapshot()
				if err != nil || !reflect.DeepEqual(raw, original) {
					t.Fatalf("dynamic hydration polluted cached Snapshot declarations: got %+v want %+v err=%v", raw, original, err)
				}
			}
			if len(lookup.requests) != 2 {
				t.Fatalf("hot cache suppressed runtime reads: %d", len(lookup.requests))
			}
			for _, request := range lookup.requests {
				if request.Binding.Repository != repositoryID || request.Binding.DeclarationCommit != commit || request.Identity != identity {
					t.Fatalf("hot cache changed runtime identity or declaration basis: %+v", request)
				}
			}
			after := cache.Stats()
			if after.SourceReads != warm.SourceReads {
				t.Fatalf("dynamic hydration caused extra Snapshot source reads: before=%+v after=%+v", warm, after)
			}
			if shape == "object" && after.Hits != warm.Hits+4 {
				t.Fatalf("test did not exercise retained Snapshot bodies on every read: before=%+v after=%+v", warm, after)
			}
		})
	}
}
