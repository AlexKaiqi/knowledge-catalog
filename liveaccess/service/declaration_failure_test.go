package service_test

import (
	"context"
	"testing"

	"kc/internal/bindingfixture"
	"kc/kernel"
	"kc/liveaccess/evidence"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
)

func TestUnavailableDeclaredDescriptorIsLocalBeforeSourceLookup(t *testing.T) {
	base, id, commit, address := setupServing(t)
	repo, err := base.Member(id)
	if err != nil {
		t.Fatal(err)
	}
	selectedRepo := &selectedStateReferenceRepo{Repository: repo, unit: objectmodel.Unit{
		ObjectID: address.ObjectID, Address: address, Digest: kernel.CanonicalDigest(nil),
		ValueSource: bindingfixture.Source("descriptor/unavailable"),
	}}
	commits := map[kernel.RepositoryID]kernel.CommitID{id: commit}
	scope := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return selectedRepo, nil },
		reader.DatasetPin{Repositories: commits, Items: reader.WholeRepositoryItems(commits)})
	lookup := &stateLookup{}
	collector := evidence.NewCollector()
	ctx := evidence.WithCollector(context.Background(), collector)
	values, err := liveservice.Open(scope, lookup, kernel.IdentityContext{Principal: "consumer"}).ReadReference(ctx,
		objectmodel.Reference{Repository: id, Object: address.ObjectID, Locator: "/runtime/status"})
	assertFailedUnitRead(t, values, err, address, kernel.ErrObjectRefUnresolved)
	samples := collectedSamples(t, collector)
	if len(samples) != 1 || samples[0].Outcome != evidence.FailureOutcome || samples[0].Observation != nil ||
		samples[0].Declaration.ValueSource.Binding.DescriptorRef != "descriptor/unavailable" || len(lookup.requests) != 0 || selectedRepo.wholeReads != 0 {
		t.Fatalf("failed declared handle lost its declaration or invoked source/object reread: %#v %#v whole=%d", samples, lookup.requests, selectedRepo.wholeReads)
	}
	raw, err := selectedRepo.ReadAddress(address, commit)
	if err != nil {
		t.Fatal(err)
	}
	result, err := liveservice.HydrateRepositoryValue(context.Background(), selectedRepo, raw, lookup, objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "maintenance"}}, nil)
	if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved || result.Value != nil || len(lookup.requests) != 0 {
		t.Fatalf("strict hydration admitted an unavailable declared handle: %#v %#v %v", result, lookup.requests, err)
	}
}
