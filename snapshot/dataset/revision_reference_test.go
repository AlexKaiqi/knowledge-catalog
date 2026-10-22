package dataset_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func referenceCoordinator(t *testing.T, service catalog.Service) catalog.RevisionReferenceCoordinator {
	t.Helper()
	port, ok := service.(catalog.RevisionReferenceCoordinator)
	if !ok {
		t.Fatal("registry has no consumer reference coordination")
	}
	return port
}

func TestRevisionReferencePreservesDefinitionAndReadsActualAuthorityAcrossPeers(t *testing.T) {
	f := newPublicationFixture(t)
	owner := referenceCoordinator(t, f.service)
	peer := referenceCoordinator(t, f.peer)
	lookup := catalog.PublicationLookup{SetID: "notes", Revision: 1}
	before := kernel.CanonicalDigest(catalog.NormalizeState(f.service.DumpState()))
	checkpoint, err := owner.PrepareRevisionReference(context.Background(), lookup, "index/main")
	if err != nil || checkpoint.Current != nil || checkpoint.ID == "" {
		t.Fatalf("first preparation: %#v %v", checkpoint, err)
	}
	value := []byte("ready-resource-1")
	accepted, err := owner.CommitRevisionReference(context.Background(), checkpoint, value)
	if err != nil {
		t.Fatal(err)
	}
	value[0] = 'x'
	accepted.Value[0] = 'y'
	actual, err := peer.RevisionReference(context.Background(), lookup, "index/main")
	if err != nil || string(actual.Value) != "ready-resource-1" || actual.ID != checkpoint.ID || actual.Authority != checkpoint.PublicationAuthority {
		t.Fatalf("peer read cached state or caller alias: %#v %v", actual, err)
	}
	if before != kernel.CanonicalDigest(catalog.NormalizeState(f.service.DumpState())) {
		t.Fatal("consumer reference changed frozen Dataset or revisions")
	}
	next, err := peer.PrepareRevisionReference(context.Background(), lookup, "index/main")
	if err != nil || next.Current == nil || next.Current.ID != checkpoint.ID || next.Head == checkpoint.Head || next.ID == checkpoint.ID {
		t.Fatalf("replacement did not fix actual authority and prior reference: %#v %v", next, err)
	}
	if _, err := peer.CommitRevisionReference(context.Background(), next, []byte("ready-resource-2")); err != nil {
		t.Fatal(err)
	}
	old, err := owner.RevisionReferenceReceipt(context.Background(), checkpoint.ID)
	if err != nil || string(old.Value) != "ready-resource-1" {
		t.Fatalf("replacement lost immutable acceptance receipt: %#v %v", old, err)
	}
	latest, err := owner.RevisionReference(context.Background(), lookup, "index/main")
	if err != nil || latest.ID != next.ID || string(latest.Value) != "ready-resource-2" {
		t.Fatalf("original process failed to read peer replacement: %#v %v", latest, err)
	}
}

func TestRevisionReferenceRejectsPausedOldRevisionAfterPeerPublication(t *testing.T) {
	f := newPublicationFixture(t)
	owner := referenceCoordinator(t, f.service)
	lookup := catalog.PublicationLookup{SetID: "notes", Revision: 1}
	checkpoint, err := owner.PrepareRevisionReference(context.Background(), lookup, "index/main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publicationCoordinator(t, f.peer).FencePublications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.peer.PublishDataset(f.candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.CommitRevisionReference(context.Background(), checkpoint, []byte("late-resource")); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("paused old producer rebased onto new revision: %v", err)
	}
	if _, err := owner.RevisionReferenceReceipt(context.Background(), checkpoint.ID); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("rejected proposal acquired receipt: %v", err)
	}
	if _, err := owner.PrepareRevisionReference(context.Background(), lookup, "index/main"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("cached old service prepared replaced revision: %v", err)
	}
}

func TestRevisionReferenceEqualPreparationsKeepDistinctAcceptanceIdentity(t *testing.T) {
	f := newPublicationFixture(t)
	owner, peer := referenceCoordinator(t, f.service), referenceCoordinator(t, f.peer)
	lookup := catalog.PublicationLookup{SetID: "notes", Revision: 1}
	first, err := owner.PrepareRevisionReference(context.Background(), lookup, "index/main")
	if err != nil {
		t.Fatal(err)
	}
	other, err := peer.PrepareRevisionReference(context.Background(), lookup, "index/main")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == other.ID || first.Head != other.Head {
		t.Fatal("equal proposals did not retain distinct identities at the same version")
	}
	if _, err := owner.CommitRevisionReference(context.Background(), first, []byte("equal")); err != nil {
		t.Fatal(err)
	}
	if _, err := peer.CommitRevisionReference(context.Background(), other, []byte("equal")); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("equal loser took over winner: %v", err)
	}
	if _, err := peer.RevisionReferenceReceipt(context.Background(), other.ID); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("equal loser has a receipt: %v", err)
	}
}

func TestRevisionReferenceLostResponseRecoversReceiptWithoutRetryingMutation(t *testing.T) {
	f := newPublicationFixture(t)
	owner, peer := referenceCoordinator(t, f.service), referenceCoordinator(t, f.peer)
	lookup := catalog.PublicationLookup{SetID: "notes", Revision: 1}
	checkpoint, err := owner.PrepareRevisionReference(context.Background(), lookup, "index/main")
	if err != nil {
		t.Fatal(err)
	}
	lost := errors.New("accepted reference response lost")
	f.a.after = func(snapshot.TreeChangeSet) error { return lost }
	if _, err := owner.CommitRevisionReference(context.Background(), checkpoint, []byte("protected-ready")); !errors.Is(err, lost) {
		t.Fatalf("lost response was hidden: %v", err)
	}
	receipt, err := peer.RevisionReferenceReceipt(context.Background(), checkpoint.ID)
	if err != nil || receipt.ID != checkpoint.ID || !bytes.Equal(receipt.Value, []byte("protected-ready")) {
		t.Fatalf("uncertain acceptance not recoverable: %#v %v", receipt, err)
	}
	if _, err := owner.CommitRevisionReference(context.Background(), checkpoint, []byte("protected-ready")); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("unknown attempt was silently rebased: %v", err)
	}
}

func TestRevisionReferenceRetirementClosesConsumptionButRetainsAcceptanceFacts(t *testing.T) {
	f := newPublicationFixture(t)
	owner := referenceCoordinator(t, f.service)
	lookup := catalog.PublicationLookup{SetID: "notes", Revision: 1}
	checkpoint, err := owner.PrepareRevisionReference(context.Background(), lookup, "index/main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.CommitRevisionReference(context.Background(), checkpoint, []byte("resource")); err != nil {
		t.Fatal(err)
	}
	if err := f.service.RetireDataset("notes"); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.RevisionReference(context.Background(), lookup, "index/main"); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
		t.Fatalf("retired reference remained consumable: %v", err)
	}
	if _, err := owner.PrepareRevisionReference(context.Background(), lookup, "index/main"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("retired reference remained replaceable: %v", err)
	}
	if receipt, err := owner.RevisionReferenceReceipt(context.Background(), checkpoint.ID); err != nil || receipt.ID != checkpoint.ID {
		t.Fatalf("retirement erased acceptance fact: %#v %v", receipt, err)
	}
}

func TestRevisionReferenceRejectsChangedBindingAndCanceledCommit(t *testing.T) {
	for _, mode := range []string{"binding", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f := newPublicationFixture(t)
			owner := referenceCoordinator(t, f.service)
			lookup := catalog.PublicationLookup{SetID: "notes", Revision: 1}
			checkpoint, err := owner.PrepareRevisionReference(context.Background(), lookup, "index/main")
			if err != nil {
				t.Fatal(err)
			}
			head, _ := f.a.Head(snapshot.DefaultRef)
			ctx := context.Background()
			if mode == "binding" {
				f.a.origin = "https://different-authority.example"
			} else {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, err = owner.CommitRevisionReference(ctx, checkpoint, []byte("must-not-apply"))
			if mode == "binding" && kernel.CodeOf(err) != kernel.ErrPreconditionFailed || mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("invalid proposal accepted: %v", err)
			}
			after, _ := f.a.Head(snapshot.DefaultRef)
			if after != head {
				t.Fatal("rejected proposal advanced authority")
			}
		})
	}
}

func TestRevisionReferenceLakeFSAuthorityCASAndPreservationAcrossPublication(t *testing.T) {
	fake := testkit.NewLakeFSFake(t)
	sourceName, authorityName := fake.NewRepo(), fake.NewRepo()
	source, err := snapshot.OpenLakeFS("kr://reference/source", fake.DSN(sourceName), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	authority, err := snapshot.OpenLakeFS("kr://reference/authority", fake.DSN(authorityName), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	registered, err := catalog.CreateSnapshotRegistry(authority, snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	stores := snapshot.NewRegistry()
	if err := stores.Add(source); err != nil {
		t.Fatal(err)
	}
	service, err := catalog.NewService(stores, registered)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterRepository(source.ID()); err != nil {
		t.Fatal(err)
	}
	sources := []catalog.DatasetSource{{Repository: source.ID(), Selector: snapshot.DefaultRef}}
	if _, err := service.DefineDataset("fixed", 1, sources); err != nil {
		t.Fatal(err)
	}
	first := referenceCoordinator(t, service)
	key := catalog.PublicationLookup{SetID: "fixed", Revision: 1}
	checkpoint, err := first.PrepareRevisionReference(context.Background(), key, "index/main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.CommitRevisionReference(context.Background(), checkpoint, []byte("fixed-observation")); err != nil {
		t.Fatal(err)
	}
	// Open a second real adapter and service over the same protocol authority.
	another, err := snapshot.OpenLakeFS(authority.ID(), fake.DSN(authorityName), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	peerRegistry, err := catalog.OpenSnapshotRegistry(another, snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := catalog.NewService(stores, peerRegistry)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := first.PrepareRevisionReference(context.Background(), key, "index/main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := peer.DefineDataset("fixed", 2, sources); err != nil {
		t.Fatal(err)
	}
	if _, err := first.CommitRevisionReference(context.Background(), paused, []byte("late")); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("real adapter accepted stale original authority: %v", err)
	}
	retained, err := referenceCoordinator(t, peer).RevisionReference(context.Background(), key, "index/main")
	if err != nil || retained.ID != checkpoint.ID || string(retained.Value) != "fixed-observation" {
		t.Fatalf("publication lost private retained reference: %#v %v", retained, err)
	}
	view, err := peer.(catalog.ReadContextBound).ReadContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := referenceCoordinator(t, view).PrepareRevisionReference(context.Background(), catalog.PublicationLookup{SetID: "fixed", Revision: 2}, "index/main"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("request read view issued mutation proposal: %v", err)
	}
}

func TestRevisionReferenceReadViewCannotEscapeItsCanceledContext(t *testing.T) {
	f := newPublicationFixture(t)
	owner := referenceCoordinator(t, f.service)
	lookup := catalog.PublicationLookup{SetID: "notes", Revision: 1}
	checkpoint, err := owner.PrepareRevisionReference(context.Background(), lookup, "index/main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.CommitRevisionReference(context.Background(), checkpoint, []byte("protected")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	view, err := f.service.(catalog.ReadContextBound).ReadContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	port := referenceCoordinator(t, view)
	if record, err := port.RevisionReference(context.Background(), lookup, "index/main"); !errors.Is(err, context.Canceled) || record.ID != "" {
		t.Fatalf("reference getter escaped request lifetime: %#v %v", record, err)
	}
	if record, err := port.RevisionReferenceReceipt(context.Background(), checkpoint.ID); !errors.Is(err, context.Canceled) || record.ID != "" {
		t.Fatalf("receipt getter escaped request lifetime: %#v %v", record, err)
	}
	if _, err := owner.RevisionReference(context.Background(), lookup, "index/main"); err != nil {
		t.Fatalf("request cancellation poisoned root: %v", err)
	}
}

func TestRevisionReferenceCorruptionFailsClosedWithoutRepair(t *testing.T) {
	for _, mode := range []string{"version", "binding", "identity", "empty-value", "unknown-field", "trailing"} {
		t.Run(mode, func(t *testing.T) {
			f := newPublicationFixture(t)
			owner := referenceCoordinator(t, f.service)
			key := catalog.PublicationLookup{SetID: "notes", Revision: 1}
			checkpoint, err := owner.PrepareRevisionReference(context.Background(), key, "index/main")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := owner.CommitRevisionReference(context.Background(), checkpoint, []byte("resource")); err != nil {
				t.Fatal(err)
			}
			path := "_revision-references/receipts/" + checkpoint.ID + ".json"
			head, err := f.a.Head(snapshot.DefaultRef)
			if err != nil {
				t.Fatal(err)
			}
			body, err := f.a.ReadFile(path, head)
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]any
			if err := json.Unmarshal(body, &object); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "version":
				object["version"] = 99
			case "binding":
				object["authority"].(map[string]any)["binding"] = "different"
			case "identity":
				object["id"] = "bad"
			case "empty-value":
				object["value"] = ""
			case "unknown-field":
				object["unexpected"] = true
			}
			body, err = json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "trailing" {
				body = append(body, []byte(` {"extra":true}`)...)
			}
			damaged, err := f.a.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: f.a.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head, Changes: []snapshot.TreeChange{{Path: path, Content: body}}})
			if err != nil {
				t.Fatal(err)
			}
			if receipt, err := owner.RevisionReferenceReceipt(context.Background(), checkpoint.ID); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || receipt.ID != "" {
				t.Fatalf("corrupt receipt accepted: %#v %v", receipt, err)
			}
			after, _ := f.a.Head(snapshot.DefaultRef)
			if after != damaged {
				t.Fatal("receipt getter repaired corrupt authority")
			}
		})
	}
}
