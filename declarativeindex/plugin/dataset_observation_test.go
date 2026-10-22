package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

type observationTicket struct {
	evidence                              []byte
	admitErr, confirmErr                  error
	onAdmit                               func()
	admitted, confirmed, canceled, closed int
}

func (p *observationTicket) Admit(_ context.Context, raw []byte) error {
	p.admitted++
	p.evidence = bytes.Clone(raw)
	if p.onAdmit != nil {
		p.onAdmit()
	}
	return p.admitErr
}
func (p *observationTicket) Confirm(context.Context) error { p.confirmed++; return p.confirmErr }
func (p *observationTicket) Cancel(context.Context) error  { p.canceled++; return nil }
func (p *observationTicket) Close() error                  { p.closed++; return nil }

func observationAuthorityFixture(t *testing.T) (datasetObservationAuthority, dataset.Service, func(time.Time)) {
	t.Helper()
	fake := testkit.NewLakeFSFake(t)
	authorityName := fake.NewRepo()
	authority, err := snapshot.OpenLakeFS("kr://observation/authority", fake.DSN(authorityName), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	registry, err := dataset.CreateSnapshotRegistry(authority, snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	sources := snapshot.NewRegistry()
	if err := sources.Add(testkit.MakeRepository(t, "kr://observation/source")); err != nil {
		t.Fatal(err)
	}
	service, err := dataset.NewService(sources, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterRepository("kr://observation/source"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DefineDataset("observed", 1, []dataset.DatasetSource{{Repository: "kr://observation/source", Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	peerStore, err := snapshot.OpenLakeFS(authority.ID(), fake.DSN(authorityName), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	peerRegistry, err := dataset.OpenSnapshotRegistry(peerStore, snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := dataset.NewService(sources, peerRegistry)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	a := datasetObservationAuthority{references: service.(dataset.RevisionReferenceCoordinator), publications: service.(dataset.PublicationCoordinator), owner: "index/main/observations", now: func() time.Time { return now }}
	return a, peer, func(value time.Time) { now = value }
}

func TestObservationAuthorityAcceptanceKeepsOriginalContinuationDeadlines(t *testing.T) {
	a, peer, clock := observationAuthorityFixture(t)
	ctx := context.Background()
	first := &observationTicket{}
	if err := publishObservation(a, ctx, "observed", 1, func() (ports.PreparedResource, error) { return first, nil }); err != nil {
		t.Fatal(err)
	}
	if first.admitted != 1 || first.confirmed != 1 || first.closed != 1 || first.canceled != 0 {
		t.Fatalf("ticket lifecycle: %#v", first)
	}
	firstAt := a.now()
	clock(firstAt.Add(time.Minute))
	second := &observationTicket{}
	if err := publishObservation(a, ctx, "observed", 1, func() (ports.PreparedResource, error) { return second, nil }); err != nil {
		t.Fatal(err)
	}
	clock(firstAt.Add(2 * time.Minute))
	third := &observationTicket{}
	if err := publishObservation(a, ctx, "observed", 1, func() (ports.PreparedResource, error) { return third, nil }); err != nil {
		t.Fatal(err)
	}
	got, err := a.evidence(ctx, "observed", 1)
	if err != nil || len(got) != 3 || !bytes.Equal(got[0].Evidence, third.evidence) || !bytes.Equal(got[1].Evidence, first.evidence) || !bytes.Equal(got[2].Evidence, second.evidence) {
		t.Fatalf("accepted generations: %q %v", got, err)
	}
	clock(firstAt.Add(3 * time.Minute)) // first expires exactly; third refresh did not extend it.
	got, err = a.evidence(ctx, "observed", 1)
	if err != nil || len(got) != 2 || !bytes.Equal(got[0].Evidence, third.evidence) || !bytes.Equal(got[1].Evidence, second.evidence) {
		t.Fatalf("original deadline changed: %q %v", got, err)
	}
	if _, err := peer.(dataset.PublicationCoordinator).FencePublications(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := peer.DefineDataset("observed", 2, []dataset.DatasetSource{{Repository: "kr://observation/source", Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	clock(firstAt.Add(20 * time.Minute))
	got, err = a.evidence(ctx, "observed", 1)
	if err != nil || len(got) != 1 || !bytes.Equal(got[0].Evidence, third.evidence) {
		t.Fatalf("retained revision lost its final accepted observation: %q %v", got, err)
	}
	if err := peer.RetireDataset("observed"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.evidence(ctx, "observed", 1); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
		t.Fatalf("retired Dataset remained queryable: %v", err)
	}
	if accepted, err := a.resolve(ctx, third.evidence); err != nil || !accepted {
		t.Fatalf("retirement erased acceptance fact: %v %v", accepted, err)
	}
}

func TestObservationAuthorityPeerFenceRejectsLateProducerWithoutDroppingProtection(t *testing.T) {
	a, peer, _ := observationAuthorityFixture(t)
	ctx := context.Background()
	first := &observationTicket{}
	if err := publishObservation(a, ctx, "observed", 1, func() (ports.PreparedResource, error) { return first, nil }); err != nil {
		t.Fatal(err)
	}
	late := &observationTicket{onAdmit: func() {
		if _, err := peer.(dataset.PublicationCoordinator).FencePublications(ctx); err != nil {
			t.Fatal(err)
		}
	}}
	err := publishObservation(a, ctx, "observed", 1, func() (ports.PreparedResource, error) { return late, nil })
	if kernel.CodeOf(err) != kernel.ErrNonFastForward || late.confirmed != 0 || late.canceled != 0 || late.closed != 1 {
		t.Fatalf("late producer or protection: %#v %v", late, err)
	}
	got, err := a.evidence(ctx, "observed", 1)
	if err != nil || len(got) != 1 || !bytes.Equal(got[0].Evidence, first.evidence) {
		t.Fatalf("unaccepted generation replaced serving reference: %q %v", got, err)
	}
	if accepted, err := a.resolve(ctx, late.evidence); err != nil || accepted {
		t.Fatalf("rejected admission recovery: %v %v", accepted, err)
	}
	if accepted, err := a.resolve(ctx, first.evidence); err != nil || !accepted {
		t.Fatalf("winner acceptance recovery: %v %v", accepted, err)
	}
}

func TestObservationAuthorityUnknownAdmissionAndConfirmationPreserveReferences(t *testing.T) {
	for _, stage := range []string{"prepare", "admit", "confirm"} {
		t.Run(stage, func(t *testing.T) {
			a, _, _ := observationAuthorityFixture(t)
			problem := errors.New("lost response")
			ticket := &observationTicket{}
			if stage == "admit" {
				ticket.admitErr = problem
			}
			if stage == "confirm" {
				ticket.confirmErr = problem
			}
			err := publishObservation(a, context.Background(), "observed", 1, func() (ports.PreparedResource, error) {
				if stage == "prepare" {
					return ticket, problem
				}
				return ticket, nil
			})
			if !errors.Is(err, problem) || ticket.closed != 1 {
				t.Fatalf("lost error/local handle: %#v %v", ticket, err)
			}
			wantCancel := 0
			if stage == "prepare" {
				wantCancel = 1
			}
			if ticket.canceled != wantCancel {
				t.Fatalf("uncertain admission canceled: %#v", ticket)
			}
			got, readErr := a.evidence(context.Background(), "observed", 1)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if stage == "confirm" {
				if len(got) != 1 || !bytes.Equal(got[0].Evidence, ticket.evidence) {
					t.Fatalf("confirmation failure withdrew acceptance: %q", got)
				}
			} else if len(got) != 0 {
				t.Fatalf("failed admission invented acceptance: %q", got)
			}
		})
	}
}

func TestObservationAuthorityRecoveryFenceExcludesPausedOriginalCAS(t *testing.T) {
	a, _, _ := observationAuthorityFixture(t)
	ctx := context.Background()
	checkpoint, err := a.references.PrepareRevisionReference(ctx, dataset.PublicationLookup{SetID: "observed", Revision: 1}, a.owner)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(observationProof(checkpoint))
	if err != nil {
		t.Fatal(err)
	}
	if accepted, err := a.resolve(ctx, raw); err != nil || accepted {
		t.Fatalf("unaccepted proof: %v %v", accepted, err)
	}
	value, err := json.Marshal(observationReferences{Version: 1, Current: raw})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.references.CommitRevisionReference(ctx, checkpoint, value); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("recovery released a still-eligible attempt: %v", err)
	}
}

func TestObservationAuthorityRejectsCorruptReferenceBeforeAllocatingResources(t *testing.T) {
	a, _, _ := observationAuthorityFixture(t)
	ctx := context.Background()
	checkpoint, err := a.references.PrepareRevisionReference(ctx, dataset.PublicationLookup{SetID: "observed", Revision: 1}, a.owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.references.CommitRevisionReference(ctx, checkpoint, []byte(`{"version":1,"current":"Zm9yZ2Vk","extra":true}`)); err != nil {
		t.Fatal(err)
	}
	allocated := false
	err = publishObservation(a, ctx, "observed", 1, func() (ports.PreparedResource, error) { allocated = true; return &observationTicket{}, nil })
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || allocated {
		t.Fatalf("corruption allocated/replaced resources: allocated=%v %v", allocated, err)
	}
	if _, err := a.evidence(ctx, "observed", 1); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("corruption became an empty or current query: %v", err)
	}
}

func TestObservationAuthorityForeignEvidenceCannotAdvanceRecoveryFence(t *testing.T) {
	a, _, _ := observationAuthorityFixture(t)
	ctx := context.Background()
	key := dataset.PublicationLookup{SetID: "observed", Revision: 1}
	checkpoint, err := a.references.PrepareRevisionReference(ctx, key, a.owner)
	if err != nil {
		t.Fatal(err)
	}
	foreign := observationProof(checkpoint)
	foreign.Owner = "index/other/observations"
	raw, err := json.Marshal(foreign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.resolve(ctx, raw); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("foreign scope was resolved: %v", err)
	}
	after, err := a.references.PrepareRevisionReference(ctx, key, a.owner)
	if err != nil || after.Head != checkpoint.Head {
		t.Fatalf("foreign evidence moved authority: %#v %v", after, err)
	}
}

func TestObservationAuthorityManagedProviderCannotOmitProtection(t *testing.T) {
	a, _, _ := observationAuthorityFixture(t)
	ctx := context.Background()
	err := publishObservation(a, ctx, "observed", 1, func() (ports.PreparedResource, error) { return nil, nil })
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing preparation ticket accepted: %v", err)
	}
	if got, err := a.evidence(ctx, "observed", 1); err != nil || len(got) != 0 {
		t.Fatalf("missing protection acquired acceptance: %#v %v", got, err)
	}
}

// Drive the same two phases as Index: fix authority before external work,
// then let acceptance own preparation and protection cleanup.
func publishObservation(a datasetObservationAuthority, ctx context.Context, setID string, revision int, prepare func() (ports.PreparedResource, error)) error {
	accept, err := a.begin(ctx, setID, revision)
	if err != nil {
		return err
	}
	return accept(prepare)
}

func TestObservationReferenceEligibilityExpiresOriginalHistoryAndRetirementWithoutErasingAcceptance(t *testing.T) {
	a, peer, clock := observationAuthorityFixture(t)
	ctx := context.Background()
	firstAt := a.now()
	first, second := &observationTicket{}, &observationTicket{}
	if err := publishObservation(a, ctx, "observed", 1, func() (ports.PreparedResource, error) { return first, nil }); err != nil {
		t.Fatal(err)
	}
	clock(firstAt.Add(time.Minute))
	if err := publishObservation(a, ctx, "observed", 1, func() (ports.PreparedResource, error) { return second, nil }); err != nil {
		t.Fatal(err)
	}
	if live, err := a.referenced(ctx, first.evidence); err != nil || !live {
		t.Fatalf("old observation lost unexpired grace: %v %v", live, err)
	}
	clock(firstAt.Add(3 * time.Minute))
	if live, err := a.referenced(ctx, first.evidence); err != nil || live {
		t.Fatalf("expired old observation remained a publication root: %v %v", live, err)
	}
	if accepted, err := a.resolve(ctx, first.evidence); err != nil || !accepted {
		t.Fatalf("expiry erased acceptance fact: %v %v", accepted, err)
	}
	if _, err := peer.(dataset.PublicationCoordinator).FencePublications(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := peer.DefineDataset("observed", 2, []dataset.DatasetSource{{Repository: "kr://observation/source", Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	clock(firstAt.Add(time.Hour))
	if live, err := a.referenced(ctx, second.evidence); err != nil || !live {
		t.Fatalf("retained revision lost its last accepted observation: %v %v", live, err)
	}
	if _, err := peer.(dataset.PublicationCoordinator).FencePublications(ctx); err != nil {
		t.Fatal(err)
	}
	if err := peer.RetireDataset("observed"); err != nil {
		t.Fatal(err)
	}
	if live, err := a.referenced(ctx, second.evidence); err != nil || live {
		t.Fatalf("known retirement retained observation publication root: %v %v", live, err)
	}
	if accepted, err := a.resolve(ctx, second.evidence); err != nil || !accepted {
		t.Fatalf("retirement erased acceptance fact: %v %v", accepted, err)
	}
}

type missingObservationLifecycleInspector struct {
	dataset.PublicationCoordinator
	inspector dataset.PublicationInspector
}

func (a missingObservationLifecycleInspector) InspectPublications(ctx context.Context, lookups ...dataset.PublicationLookup) (dataset.PublicationCheckpoint, error) {
	checkpoint, err := a.inspector.InspectPublications(ctx, lookups...)
	checkpoint.State.Datasets = nil
	return checkpoint, err
}
func TestObservationReferenceMissingLifecycleIsUnknownAndInspectionDoesNotWrite(t *testing.T) {
	a, _, _ := observationAuthorityFixture(t)
	ctx := context.Background()
	ticket := &observationTicket{}
	if err := publishObservation(a, ctx, "observed", 1, func() (ports.PreparedResource, error) { return ticket, nil }); err != nil {
		t.Fatal(err)
	}
	inspector := a.publications.(dataset.PublicationInspector)
	before, err := inspector.InspectPublications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if live, err := a.referenced(ctx, ticket.evidence); err != nil || !live {
		t.Fatalf("accepted observation: %v %v", live, err)
	}
	after, err := inspector.InspectPublications(ctx)
	if err != nil || after.Authority.Head != before.Authority.Head {
		t.Fatalf("observation reconciliation advanced authority: %#v %v", after, err)
	}
	a.publications = missingObservationLifecycleInspector{PublicationCoordinator: a.publications, inspector: inspector}
	if live, err := a.referenced(ctx, ticket.evidence); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || live {
		t.Fatalf("missing lifecycle became retirement: %v %v", live, err)
	}
}
