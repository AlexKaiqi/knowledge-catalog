package datasets

import (
	"context"
	"encoding/json"
	"errors"
	"kc/snapshot"
	"testing"
	"time"

	"kc/kernel"
	"kc/snapshot/dataset"
)

func capturePublicationEvidence(t *testing.T, service dataset.Service, fixture publicationFailureFixture, publish bool) (dataset.Dataset, []byte) {
	t.Helper()
	def, err := service.PrepareDataset("reading", 2, fixture.sources)
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	stopped := errors.New("captured before authority CAS")
	_, err = service.(dataset.PublicationCoordinator).PublishDatasetGuarded(context.Background(), def, func(_ context.Context, authority dataset.PublicationAdmission) error {
		raw, err = json.Marshal(publicationResourceEvidence{Version: 2, Authority: authority.PublicationAuthority, Admission: authority.ID, Definition: def})
		if err != nil {
			return err
		}
		if publish {
			return nil
		}
		return stopped
	})
	if (publish && err != nil) || (!publish && !errors.Is(err, stopped)) {
		t.Fatal(err)
	}
	return def, raw
}

func TestPublicationRecoveryReadsAcceptedHistoryInsteadOfCurrentPointer(t *testing.T) {
	fixture := newPublicationFailureFixture(t)
	service := guardedPublicationService(t, fixture)
	_, raw := capturePublicationEvidence(t, service, fixture, true)
	if _, err := service.DefineDataset("reading", 3, fixture.sources); err != nil {
		t.Fatal(err)
	}
	before, _ := fixture.authority.Head(snapshot.DefaultRef)
	accepted, err := (PublicationRecovery{Authority: service.(dataset.PublicationCoordinator)}).Resolve(context.Background(), raw)
	if err != nil || !accepted {
		t.Fatalf("accepted old revision was lost: accepted=%t err=%v", accepted, err)
	}
	after, _ := fixture.authority.Head(snapshot.DefaultRef)
	if after == before {
		t.Fatal("recovery did not establish a new authority fence")
	}
	current, _ := service.Set("reading")
	if current.Revision != 3 {
		t.Fatal("recovery changed current Dataset revision")
	}
}

func TestPublicationRecoveryFencesDelayedPublisherBeforeReleasingNonAdoption(t *testing.T) {
	fixture := newPublicationFailureFixture(t)
	service := guardedPublicationService(t, fixture)
	def, err := service.PrepareDataset("reading", 2, fixture.sources)
	if err != nil {
		t.Fatal(err)
	}
	proofs := make(chan []byte, 1)
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	done := make(chan error, 1)
	go func() {
		_, err := service.(dataset.PublicationCoordinator).PublishDatasetGuarded(context.Background(), def, func(_ context.Context, authority dataset.PublicationAdmission) error {
			raw, err := json.Marshal(publicationResourceEvidence{Version: 2, Authority: authority.PublicationAuthority, Admission: authority.ID, Definition: def})
			if err != nil {
				return err
			}
			proofs <- raw
			<-release
			return nil
		})
		done <- err
	}()
	var raw []byte
	select {
	case raw = <-proofs:
	case err := <-done:
		t.Fatalf("publisher failed before admission: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("publisher did not reach admission")
	}
	accepted, recoveryErr := (PublicationRecovery{Authority: service.(dataset.PublicationCoordinator)}).Resolve(context.Background(), raw)
	close(release)
	publishErr := <-done
	if recoveryErr != nil || accepted || kernel.CodeOf(publishErr) != kernel.ErrNonFastForward {
		t.Fatalf("late publisher crossed recovery fence: accepted=%t recovery=%v publish=%v", accepted, recoveryErr, publishErr)
	}
	if _, err := service.DatasetVersion("reading", 2); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("rejected revision was accepted: %v", err)
	}
}

func TestPublicationRecoveryMatchesFullIntentAndAuthority(t *testing.T) {
	for _, variant := range []string{"different-candidate", "different-binding", "different-ref", "different-authority"} {
		t.Run(variant, func(t *testing.T) {
			fixture := newPublicationFailureFixture(t)
			service := guardedPublicationService(t, fixture)
			def, raw := capturePublicationEvidence(t, service, fixture, false)
			var proof publicationResourceEvidence
			if err := json.Unmarshal(raw, &proof); err != nil {
				t.Fatal(err)
			}
			switch variant {
			case "different-candidate":
				def.Metadata = map[string]string{"different": "intent"}
			case "different-binding":
				proof.Authority.Binding = "foreign-instance"
			case "different-ref":
				proof.Authority.Ref = "refs/heads/other"
			case "different-authority":
				proof.Authority.AuthorityID = "kr://foreign/registry"
			}
			if _, err := service.PublishDataset(def); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(proof)
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := (PublicationRecovery{Authority: service.(dataset.PublicationCoordinator)}).Resolve(context.Background(), raw)
			if accepted || ((err != nil) != (variant != "different-candidate")) {
				t.Fatalf("mismatched publication got completion proof: accepted=%t err=%v", accepted, err)
			}
		})
	}
}

func TestPublicationRecoveryRetainsUnknownAuthorityAndMalformedEvidence(t *testing.T) {
	fixture := newPublicationFailureFixture(t)
	service := guardedPublicationService(t, fixture)
	_, raw := capturePublicationEvidence(t, service, fixture, false)
	unavailable := errors.New("authority fence confirmation lost")
	service.SetObserver(func(string, map[string]any, error) error { return unavailable })
	if accepted, err := (PublicationRecovery{Authority: service.(dataset.PublicationCoordinator)}).Resolve(context.Background(), raw); accepted || !errors.Is(err, unavailable) {
		t.Fatalf("unknown fence allowed release: %t %v", accepted, err)
	}
	service.SetObserver(nil)
	before, _ := fixture.authority.Head(snapshot.DefaultRef)
	for _, raw := range [][]byte{nil, []byte(`{}`), []byte(`{"version":99}`), append(append([]byte{}, raw...), []byte(` {}`)...)} {
		if accepted, err := (PublicationRecovery{Authority: service.(dataset.PublicationCoordinator)}).Resolve(context.Background(), raw); accepted || kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("bad evidence produced recovery outcome: %t %v", accepted, err)
		}
	}
	if after, _ := fixture.authority.Head(snapshot.DefaultRef); after != before {
		t.Fatal("malformed evidence entered authority mutation")
	}
}

func TestPublicationRecoveryDistinguishesEqualPreparationAttempts(t *testing.T) {
	fixture := newPublicationFailureFixture(t)
	service := guardedPublicationService(t, fixture)
	def, err := service.PrepareDataset("reading", 2, fixture.sources)
	if err != nil {
		t.Fatal(err)
	}
	var evidence [2]chan []byte
	var release [2]chan struct{}
	var done [2]chan error
	for i := range evidence {
		evidence[i], release[i], done[i] = make(chan []byte, 1), make(chan struct{}), make(chan error, 1)
		t.Cleanup(func() {
			select {
			case <-release[i]:
			default:
				close(release[i])
			}
		})
		go func() {
			_, err := service.(dataset.PublicationCoordinator).PublishDatasetGuarded(context.Background(), def, func(_ context.Context, admission dataset.PublicationAdmission) error {
				raw, err := json.Marshal(publicationResourceEvidence{Version: 2, Admission: admission.ID, Authority: admission.PublicationAuthority, Definition: def})
				if err != nil {
					return err
				}
				evidence[i] <- raw
				<-release[i]
				return nil
			})
			done[i] <- err
		}()
	}
	var raw [2][]byte
	for i := range raw {
		select {
		case raw[i] = <-evidence[i]:
		case err := <-done[i]:
			t.Fatalf("publication missed admission: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("admission did not arrive")
		}
	}
	close(release[0])
	if err := <-done[0]; err != nil {
		t.Fatal(err)
	}
	close(release[1])
	if err := <-done[1]; kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("equal second publication was accepted: %v", err)
	}
	recovery := PublicationRecovery{Authority: service.(dataset.PublicationCoordinator)}
	for i := range raw {
		adopted, err := recovery.Resolve(context.Background(), raw[i])
		if err != nil || adopted != (i == 0) {
			t.Fatalf("equal logical candidates lost actual winner: attempt=%d adopted=%t err=%v", i, adopted, err)
		}
	}
}

func TestPublicationRecoveryCannotInferLegacyPreparationFromEqualDefinition(t *testing.T) {
	fixture := newPublicationFailureFixture(t)
	service := guardedPublicationService(t, fixture)
	_, raw := capturePublicationEvidence(t, service, fixture, true)
	var proof publicationResourceEvidence
	if err := json.Unmarshal(raw, &proof); err != nil {
		t.Fatal(err)
	}
	proof.Version, proof.Admission = 1, ""
	legacy, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := (PublicationRecovery{Authority: service.(dataset.PublicationCoordinator)}).Resolve(context.Background(), legacy)
	if accepted || kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("legacy evidence invented a preparation identity: %t %v", accepted, err)
	}
}

func TestPublicationReferenceEligibilitySeparatesRetainedAndRetiredFromAcceptance(t *testing.T) {
	fixture := newPublicationFailureFixture(t)
	service := guardedPublicationService(t, fixture)
	_, raw := capturePublicationEvidence(t, service, fixture, true)
	recovery := PublicationRecovery{Authority: service.(dataset.PublicationCoordinator)}
	if _, err := service.DefineDataset("reading", 3, fixture.sources); err != nil {
		t.Fatal(err)
	}
	if live, err := recovery.Referenced(context.Background(), raw); err != nil || !live {
		t.Fatalf("retained definition lost reference: %v %v", live, err)
	}
	if err := service.RetireDataset("reading"); err != nil {
		t.Fatal(err)
	}
	if live, err := recovery.Referenced(context.Background(), raw); err != nil || live {
		t.Fatalf("retired definition still referenced: %v %v", live, err)
	}
	if accepted, err := recovery.Resolve(context.Background(), raw); err != nil || !accepted {
		t.Fatalf("retirement erased acceptance fact: %v %v", accepted, err)
	}
}

type missingLifecycleInspector struct {
	dataset.PublicationCoordinator
	inspector dataset.PublicationInspector
}

func (a missingLifecycleInspector) InspectPublications(ctx context.Context, lookups ...dataset.PublicationLookup) (dataset.PublicationCheckpoint, error) {
	checkpoint, err := a.inspector.InspectPublications(ctx, lookups...)
	checkpoint.State.Datasets = nil
	return checkpoint, err
}

func TestPublicationReferenceMissingLifecycleIsUnknownAndInspectionDoesNotWrite(t *testing.T) {
	fixture := newPublicationFailureFixture(t)
	service := guardedPublicationService(t, fixture)
	_, raw := capturePublicationEvidence(t, service, fixture, true)
	authority := service.(dataset.PublicationCoordinator)
	before, _ := fixture.authority.Head(snapshot.DefaultRef)
	if live, err := (PublicationRecovery{Authority: authority}).Referenced(context.Background(), raw); err != nil || !live {
		t.Fatalf("accepted reference: %v %v", live, err)
	}
	after, _ := fixture.authority.Head(snapshot.DefaultRef)
	if after != before {
		t.Fatal("live-reference reconciliation advanced authority")
	}
	broken := missingLifecycleInspector{PublicationCoordinator: authority, inspector: service.(dataset.PublicationInspector)}
	if live, err := (PublicationRecovery{Authority: broken}).Referenced(context.Background(), raw); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || live {
		t.Fatalf("missing lifecycle became retirement: %v %v", live, err)
	}
}

func TestPublicationReferenceInspectionCannotDischargeStillEligiblePausedPublisher(t *testing.T) {
	fixture := newPublicationFailureFixture(t)
	service := guardedPublicationService(t, fixture)
	def, err := service.PrepareDataset("reading", 2, fixture.sources)
	if err != nil {
		t.Fatal(err)
	}
	proofs := make(chan []byte, 1)
	resume := make(chan struct{})
	done := make(chan error, 1)
	finished := false
	t.Cleanup(func() {
		select {
		case <-resume:
		default:
			close(resume)
		}
		if !finished {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("paused publisher did not exit")
			}
		}
	})
	go func() {
		_, err := service.(dataset.PublicationCoordinator).PublishDatasetGuarded(context.Background(), def, func(_ context.Context, admission dataset.PublicationAdmission) error {
			raw, err := marshalPublicationEvidence(def, admission)
			if err != nil {
				return err
			}
			proofs <- raw
			<-resume
			return nil
		})
		done <- err
	}()
	var raw []byte
	select {
	case raw = <-proofs:
	case err := <-done:
		finished = true
		t.Fatalf("no admission: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("admission timeout")
	}
	before, _ := fixture.authority.Head(snapshot.DefaultRef)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	live, err := (PublicationRecovery{Authority: service.(dataset.PublicationCoordinator)}).Referenced(ctx, raw)
	after, _ := fixture.authority.Head(snapshot.DefaultRef)
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || live || after != before {
		t.Fatalf("inspection discharged or fenced a still-eligible original CAS: live=%t %v", live, err)
	}
	close(resume)
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatalf("inspection invalidated original publisher: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("publication did not resume")
	}
}
