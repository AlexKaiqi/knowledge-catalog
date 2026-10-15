package service_test

import (
	"context"
	"fmt"
	"testing"

	"kc/kernel"
	"kc/liveaccess"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/snapshot"
)

type repositorySiblingGuard struct {
	snapshot.Store
	snapshot.TreeStore
	siblingReads int
}

func (g *repositorySiblingGuard) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	if path == "poison/sibling.yaml" {
		g.siblingReads++
		return nil, fmt.Errorf("selected read touched a poison sibling")
	}
	return g.TreeStore.ReadFile(path, at)
}

func (g *repositorySiblingGuard) ListFiles(kernel.CommitID) ([]string, error) {
	return nil, fmt.Errorf("selected read scanned the repository")
}

func siblingReadWrapper(guard **repositorySiblingGuard) func(snapshot.Store) snapshot.Store {
	return func(store snapshot.Store) snapshot.Store {
		*guard = &repositorySiblingGuard{Store: store, TreeStore: store.(snapshot.TreeStore)}
		return *guard
	}
}

func TestRepositoryReadAdoptsNewMemberLeafBeforeSelectingWithoutSiblingReads(t *testing.T) {
	for _, kind := range []string{"address", "new-leaf"} {
		t.Run(kind, func(t *testing.T) {
			var guard *repositorySiblingGuard
			_, repo, commit, id := memberUnitFixture(t, siblingReadWrapper(&guard))
			address := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: id, AspectName: "readings", MemberKey: "good"}
			request := liveservice.ReadRequest{Commit: commit, Address: &address}
			want := any(map[string]any{"value": float64(47), "fresh": float64(99)})
			if kind == "new-leaf" {
				request.Address = nil
				request.Reference = &objectmodel.Reference{Repository: repo.ID(), Object: id, Locator: "/readings/good/fresh"}
				want = float64(99)
			}
			lookup := &unitLookup{}
			actualLookup := lookupFunc(func(ctx context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
				observation, err := lookup.LookupState(ctx, request)
				observation.Value.(map[string]any)["fresh"] = float64(99)
				return observation, err
			})
			result, err := liveservice.ReadRepository(context.Background(), repo, request, actualLookup,
				objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "consumer"}})
			if err != nil || kernel.CanonicalDigest(result.Value) != kernel.CanonicalDigest(want) || result.Address != address ||
				len(result.Observations) != 1 || len(lookup.requests) != 1 || guard.siblingReads != 0 {
				t.Fatalf("selected Member read failed before dynamic selection or touched siblings: %#v lookups=%d sibling=%d %v", result, len(lookup.requests), guard.siblingReads, err)
			}
		})
	}
}

func TestRepositoryReadVirtualAspectAvoidsUnselectedSiblingBodies(t *testing.T) {
	for _, kind := range []string{"address", "reference"} {
		t.Run(kind, func(t *testing.T) {
			var guard *repositorySiblingGuard
			_, repo, commit, id := mixedUnitFixture(t, siblingReadWrapper(&guard))
			address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "good"}
			request := liveservice.ReadRequest{Commit: commit, Address: &address}
			want := any(map[string]any{"value": float64(31)})
			if kind == "reference" {
				request.Address = nil
				request.Reference = &objectmodel.Reference{Repository: repo.ID(), Object: id, Locator: "/good/value"}
				want = float64(31)
			}
			lookup := &unitLookup{}
			result, err := liveservice.ReadRepository(context.Background(), repo, request, lookup,
				objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "consumer"}})
			if err != nil || kernel.CanonicalDigest(result.Value) != kernel.CanonicalDigest(want) || result.Address != address ||
				len(result.Observations) != 1 || len(lookup.requests) != 1 || guard.siblingReads != 0 {
				t.Fatalf("virtual Aspect needed a Snapshot instance or sibling body: %#v lookups=%d sibling=%d %v", result, len(lookup.requests), guard.siblingReads, err)
			}
		})
	}
}

func TestRepositoryReadPreservesStaticReferenceAndRejectsConflictingSelection(t *testing.T) {
	_, repo, commit, id := mixedUnitFixture(t)
	ref := objectmodel.Reference{Repository: repo.ID(), Object: id, Locator: "/definition/owner"}
	fixed, err := repo.(objectmodel.ReferenceReader).ReadReference(ref, commit)
	if err != nil {
		t.Fatal(err)
	}
	lookup := &unitLookup{}
	request := objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "consumer"}}
	result, err := liveservice.ReadRepository(context.Background(), repo, liveservice.ReadRequest{Commit: commit, Reference: &ref}, lookup, request)
	if err != nil || kernel.CanonicalDigest(result.Value) != kernel.CanonicalDigest(fixed.Value) || result.Address != fixed.Address || result.Digest != fixed.Digest ||
		result.Reference != fixed.Reference || result.Locator != fixed.Locator || len(result.Observations) != 0 || len(lookup.requests) != 0 {
		t.Fatalf("static reference changed shape or performed source lookup: %#v %v", result, err)
	}
	address := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: id, AspectName: "readings", MemberKey: "good"}
	foreign := ref
	foreign.Repository = "kr://foreign"
	for _, selection := range []liveservice.ReadRequest{
		{Reference: &ref},
		{Commit: commit, Reference: &foreign},
		{Commit: commit, Reference: &ref, Address: &address},
		{Commit: commit, Reference: &ref, Selector: &objectmodel.AspectSelector{}},
		{Commit: commit, Object: "fixture/other", Address: &address},
		{Commit: commit, Address: &objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: id}},
	} {
		_, err := liveservice.ReadRepository(context.Background(), repo, selection, lookup, request)
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(lookup.requests) != 0 {
			t.Fatalf("invalid selection executed its source: %#v calls=%d %v", selection, len(lookup.requests), err)
		}
	}
}
