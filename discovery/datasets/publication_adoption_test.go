package datasets

import (
	"context"
	"errors"
	"testing"

	"kc/kernel"
	"kc/snapshot/dataset"
)

func TestLegacyPreparationAdoptionProtectsUnknownOutcomesAndRejectsMutation(t *testing.T) {
	for _, phase := range []string{"accepted", "unmanaged", "prepare-failed", "mutated", "admission-lost", "authority-rejected", "confirmation-lost"} {
		t.Run(phase, func(t *testing.T) {
			fixture := newPublicationFailureFixture(t)
			service := guardedPublicationService(t, fixture)
			authority := service.(dataset.PublicationCoordinator)
			def, err := service.DatasetVersion("reading", 1)
			if err != nil {
				t.Fatal(err)
			}
			before := kernel.CanonicalDigest(def)
			failed := errors.New("injected " + phase)
			ticket := &trackedPublicationProtection{admissionResource: admissionResource{admit: func(_ context.Context, actual dataset.Dataset, _ dataset.PublicationAuthority) error {
				if kernel.CanonicalDigest(actual) != before {
					t.Fatal("resource evidence changed immutable release")
				}
				if phase == "authority-rejected" {
					fixture.authority.err = failed
				}
				return nil
			}}}
			if phase == "admission-lost" {
				ticket.admitErr = failed
			}
			if phase == "confirmation-lost" {
				ticket.confirmErr = failed
			}
			recovery := PublicationRecovery{Authority: authority, Definitions: service}
			err = recovery.Adopt(context.Background(), dataset.PublicationLookup{SetID: "reading", Revision: 1}, func(_ context.Context, candidate dataset.Dataset) (PublicationProtection, error) {
				if phase == "unmanaged" {
					return nil, nil
				}
				if phase == "prepare-failed" {
					return ticket, failed
				}
				if phase == "mutated" {
					candidate.Sources[0].Commit = "changed-after-freeze"
				}
				return ticket, nil
			})
			accepted := phase == "accepted" || phase == "confirmation-lost"
			if (err == nil) != (phase == "accepted" || phase == "unmanaged") {
				t.Fatalf("adoption outcome: %v", err)
			}
			if phase == "mutated" && kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("mutated candidate not rejected: %v", err)
			}
			receipt, readErr := authority.PublicationReceipt(context.Background(), dataset.PublicationLookup{SetID: "reading", Revision: 1})
			if readErr != nil || (receipt.Admission != "") != accepted {
				t.Fatalf("adoption receipt: %#v %v", receipt, readErr)
			}
			actual, readErr := service.DatasetVersion("reading", 1)
			if readErr != nil || kernel.CanonicalDigest(actual) != before {
				t.Fatalf("adoption changed immutable release: %#v %v", actual, readErr)
			}
			cancellations := 0
			if phase == "prepare-failed" || phase == "mutated" {
				cancellations = 1
			}
			confirmations := 0
			if accepted {
				confirmations = 1
			}
			closes := 1
			if phase == "unmanaged" {
				closes = 0
			}
			if ticket.cancellations != cancellations || ticket.confirmations != confirmations || ticket.closes != closes {
				t.Fatalf("protection transition: %#v", ticket)
			}
		})
	}
}
