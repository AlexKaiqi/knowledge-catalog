package datasets

import (
	"context"
	"errors"
	"testing"

	"kc/kernel"
	"kc/snapshot/dataset"
)

type trackedPublicationProtection struct {
	admissionResource
	admitErr, confirmErr                             error
	admissions, confirmations, cancellations, closes int
}

func (r *trackedPublicationProtection) Admit(ctx context.Context, raw []byte) error {
	r.admissions++
	if r.admitErr != nil {
		return r.admitErr
	}
	return r.admissionResource.Admit(ctx, raw)
}
func (r *trackedPublicationProtection) Confirm(context.Context) error {
	r.confirmations++
	return r.confirmErr
}
func (r *trackedPublicationProtection) Cancel(context.Context) error { r.cancellations++; return nil }
func (r *trackedPublicationProtection) Close() error                 { r.closes++; return nil }

func TestDatasetPublicationResourceProtectionFollowsActualAcceptance(t *testing.T) {
	for _, phase := range []string{"accepted", "admission-response-lost", "registry-rejected", "acceptance-evidence-lost", "confirmation-lost"} {
		t.Run(phase, func(t *testing.T) {
			fixture := newPublicationFailureFixture(t)
			service := guardedPublicationService(t, fixture)
			failed := errors.New("injected " + phase)
			ticket := &trackedPublicationProtection{admissionResource: admissionResource{admit: func(_ context.Context, def dataset.Dataset, authority dataset.PublicationAuthority) error {
				if def.Revision != 2 || def.SetID != "reading" || len(def.Sources) != 1 || def.Sources[0].Commit != fixture.nextCommit || authority.Binding == "" {
					t.Fatalf("resource proof lost exact candidate: %#v %#v", def, authority)
				}
				return nil
			}}}
			switch phase {
			case "admission-response-lost":
				ticket.admitErr = failed
			case "registry-rejected":
				fixture.authority.err = kernel.Fail(kernel.ErrNonFastForward, "injected registry rejection")
			case "acceptance-evidence-lost":
				service.SetObserver(func(string, map[string]any, error) error { return failed })
			case "confirmation-lost":
				ticket.confirmErr = failed
			}
			publisher := DatasetPublisher{Registry: service, Authorize: func(context.Context, DatasetPublication) error { return nil },
				PrepareResources: func(_ context.Context, def dataset.Dataset) (dataset.Dataset, PublicationProtection, error) {
					return def, ticket, nil
				}}
			_, err := publisher.Execute(context.Background(), DatasetPublication{Dataset: "reading", Revision: 2, Sources: fixture.sources})
			wantAccepted := phase == "accepted" || phase == "confirmation-lost" || phase == "acceptance-evidence-lost"
			current, getErr := service.Set("reading")
			if getErr != nil || (current.Revision == 2) != wantAccepted {
				t.Fatalf("actual acceptance: %#v %v", current, getErr)
			}
			if (err == nil) != (phase == "accepted") {
				t.Fatalf("unexpected publication outcome: %v", err)
			}
			wantConfirmation := 0
			if phase == "accepted" || phase == "confirmation-lost" {
				wantConfirmation = 1
			}
			if ticket.admissions != 1 || ticket.confirmations != wantConfirmation || ticket.cancellations != 0 || ticket.closes != 1 {
				t.Fatalf("resource protection incorrectly released or confirmed: %+v", ticket)
			}
		})
	}
}

func TestDatasetPublicationCancelsOnlyBeforeAdmissionAttempt(t *testing.T) {
	for _, phase := range []string{"prepare-failed", "candidate-changed", "cancelled-before-admission"} {
		t.Run(phase, func(t *testing.T) {
			fixture := newPublicationFailureFixture(t)
			service := guardedPublicationService(t, fixture)
			ticket := &trackedPublicationProtection{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			publisher := DatasetPublisher{Registry: service, Authorize: func(context.Context, DatasetPublication) error { return nil },
				PrepareResources: func(_ context.Context, def dataset.Dataset) (dataset.Dataset, PublicationProtection, error) {
					switch phase {
					case "prepare-failed":
						return def, ticket, errors.New("preparation failed")
					case "candidate-changed":
						def.Revision++
					case "cancelled-before-admission":
						cancel()
					}
					return def, ticket, nil
				}}
			if _, err := publisher.Execute(ctx, DatasetPublication{Dataset: "reading", Revision: 2, Sources: fixture.sources}); err == nil {
				t.Fatal("invalid preparation accepted")
			}
			if ticket.admissions != 0 || ticket.confirmations != 0 || ticket.cancellations != 1 || ticket.closes != 1 {
				t.Fatalf("unadmitted ticket lifecycle: %+v", ticket)
			}
			current, _ := service.Set("reading")
			if current.Revision != 1 || fixture.authority.attempts != 0 {
				t.Fatal("invalid preparation reached authority CAS")
			}
		})
	}
}
