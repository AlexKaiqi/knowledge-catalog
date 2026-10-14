package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestDatasetReferenceReadKeepsSourceAndAuthorizationOrder(t *testing.T) {
	ref := objectmodel.Reference{Repository: "kr://app/read", Object: "one", Locator: "/columns/id/type"}
	want := []objectmodel.ObservedValue{{FederatedValue: objectmodel.FederatedValue{
		Repository: ref.Repository, Commit: "accepted", ObjectID: ref.Object, Locator: ref.Locator, Value: "string",
	}}}
	var order []string
	executor := DatasetReadExecutor{
		Authorize: func(context.Context) error { order = append(order, "authorize"); return nil },
		ReferenceRead: func(_ context.Context, got objectmodel.Reference) ([]objectmodel.ObservedValue, error) {
			order = append(order, "read")
			if got != ref {
				t.Fatalf("Dataset replaced source identity or locator: %#v", got)
			}
			return want, nil
		},
		Deliver: func(_ context.Context, values []objectmodel.ObservedValue) ([]objectmodel.ObservedValue, error) {
			order = append(order, "deliver")
			return values, nil
		},
	}
	got, err := executor.Execute(context.Background(), DatasetReadRequest{Reference: ref})
	if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(order, []string{"authorize", "read", "deliver"}) {
		t.Fatalf("Dataset changed fixed source or delivery order: values=%#v order=%v error=%v", got, order, err)
	}
}

func TestDatasetReferenceReadRequiresCompleteServicesBeforeAccess(t *testing.T) {
	ref := objectmodel.Reference{Repository: "kr://app/read", Object: "one"}
	for _, missing := range []string{"authorize", "read", "deliver"} {
		t.Run(missing, func(t *testing.T) {
			calls := 0
			executor := DatasetReadExecutor{
				Authorize: func(context.Context) error { calls++; return nil },
				ReferenceRead: func(context.Context, objectmodel.Reference) ([]objectmodel.ObservedValue, error) {
					calls++
					return nil, nil
				},
				Deliver: func(_ context.Context, values []objectmodel.ObservedValue) ([]objectmodel.ObservedValue, error) {
					calls++
					return values, nil
				},
			}
			switch missing {
			case "authorize":
				executor.Authorize = nil
			case "read":
				executor.ReferenceRead = nil
			case "deliver":
				executor.Deliver = nil
			}
			_, err := executor.Execute(context.Background(), DatasetReadRequest{Reference: ref})
			if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || calls != 0 {
				t.Fatalf("incomplete service reached access: %v calls=%d", err, calls)
			}
		})
	}
}

func TestDatasetReferenceReadRejectsMissingIdentityWithoutIO(t *testing.T) {
	for _, ref := range []objectmodel.Reference{{}, {Repository: "kr://app/read"}, {Object: "one"}, {Repository: "kr://app/read", Object: "one", Locator: "/~3bad"}} {
		calls := 0
		executor := DatasetReadExecutor{
			Authorize: func(context.Context) error { calls++; return nil },
			ReferenceRead: func(context.Context, objectmodel.Reference) ([]objectmodel.ObservedValue, error) {
				calls++
				return nil, nil
			},
			Deliver: func(_ context.Context, values []objectmodel.ObservedValue) ([]objectmodel.ObservedValue, error) {
				calls++
				return values, nil
			},
		}
		_, err := executor.Execute(context.Background(), DatasetReadRequest{Reference: ref})
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid || calls != 0 {
			t.Fatalf("missing reference reached access: reference=%#v error=%v calls=%d", ref, err, calls)
		}
	}
}

func TestDatasetReferenceReadFailureCannotReachDelivery(t *testing.T) {
	failure := errors.New("fixed source unavailable")
	reads, deliveries := 0, 0
	executor := DatasetReadExecutor{
		Authorize: func(context.Context) error { return nil },
		ReferenceRead: func(context.Context, objectmodel.Reference) ([]objectmodel.ObservedValue, error) {
			reads++
			return nil, failure
		},
		Deliver: func(_ context.Context, values []objectmodel.ObservedValue) ([]objectmodel.ObservedValue, error) {
			deliveries++
			return values, nil
		},
	}
	_, err := executor.Execute(context.Background(), DatasetReadRequest{Reference: objectmodel.Reference{Repository: "kr://app/read", Object: "one"}})
	if !errors.Is(err, failure) || reads != 1 || deliveries != 0 {
		t.Fatalf("failed source was delivered or error changed: %v reads=%d deliveries=%d", err, reads, deliveries)
	}
}
