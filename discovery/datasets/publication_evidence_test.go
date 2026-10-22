package datasets

import (
	"bytes"
	"context"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestPublicationEvidenceReconstructsExactAcceptedPreparation(t *testing.T) {
	fixture := newPublicationFailureFixture(t)
	service := guardedPublicationService(t, fixture)
	_, raw := capturePublicationEvidence(t, service, fixture, true)
	if _, err := service.DefineDataset("reading", 3, fixture.sources); err != nil {
		t.Fatal(err)
	}
	before, _ := fixture.authority.Head(snapshot.DefaultRef)
	lookup := PublicationRecovery{Authority: service.(dataset.PublicationCoordinator), Definitions: service}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reconstructed, err := lookup.Evidence(ctx, "reading", 2)
	if err != nil || !bytes.Equal(raw, reconstructed) {
		t.Fatalf("fixed publication identity changed after replacement: %q %v", reconstructed, err)
	}
	if after, _ := fixture.authority.Head(snapshot.DefaultRef); after != before {
		t.Fatal("query evidence lookup performed a recovery fence")
	}
}

func TestLegacyPublicationEvidenceRequiresActualAuthorityAndExistingRelease(t *testing.T) {
	fixture := newPublicationFailureFixture(t)
	service := guardedPublicationService(t, fixture)
	lookup := PublicationRecovery{Authority: service.(dataset.PublicationCoordinator), Definitions: service}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if raw, err := lookup.Evidence(ctx, "reading", 1); err != nil || len(raw) != 0 {
		t.Fatalf("valid legacy release was not recognized: %q %v", raw, err)
	}
	if _, err := lookup.Evidence(ctx, "reading", 9); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("missing release was treated as legacy: %v", err)
	}
	head, err := fixture.authority.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := fixture.authority.ListFiles(head)
	if err != nil {
		t.Fatal(err)
	}
	changes := make([]snapshot.TreeChange, 0, len(paths))
	for _, path := range paths {
		changes = append(changes, snapshot.TreeChange{Path: path, Remove: true})
	}
	if _, err := fixture.authority.TreeStore.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: fixture.authority.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head, Changes: changes, Message: "erase test authority metadata"}); err != nil {
		t.Fatal(err)
	}
	if raw, err := lookup.Evidence(ctx, "reading", 1); err == nil || len(raw) != 0 {
		t.Fatalf("erased authority was treated as a valid receipt-free release: %q %v", raw, err)
	}
}
