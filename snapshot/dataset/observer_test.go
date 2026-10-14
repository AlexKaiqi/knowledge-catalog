package dataset_test

import (
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func TestDatasetObserverPreservesAuthorityOutcome(t *testing.T) {
	for _, operation := range []string{"register", "publish"} {
		for _, outcome := range []string{"rejected-observer-nil", "rejected-observer-error", "accepted-observer-error"} {
			t.Run(operation+"/"+outcome, func(t *testing.T) {
				const source = "kr://observer/source"
				const other = "kr://observer/other"
				authority, registry := createCatalogRegistry(t, "kr://observer/registry")
				service := catalogFromRegistry(t, registry, source, other)
				if err := service.RegisterRepository(source); err != nil {
					t.Fatal(err)
				}
				if _, err := service.DefineDataset("delivery", 1, []catalog.DatasetSource{{Repository: source, Selector: snapshot.DefaultRef}}); err != nil {
					t.Fatal(err)
				}
				candidate, err := service.PrepareDataset("delivery", 2, []catalog.DatasetSource{{Repository: source, Selector: snapshot.DefaultRef}})
				if err != nil {
					t.Fatal(err)
				}
				before := catalog.NormalizeState(service.DumpState())
				head, err := registry.Head()
				if err != nil {
					t.Fatal(err)
				}
				domainErr := kernel.Fail(kernel.ErrNonFastForward, "registry rejected publication")
				recordErr := kernel.Fail(kernel.ErrTemporaryUnavailable, "observer record unavailable")
				rejected := outcome != "accepted-observer-error"
				if rejected {
					authority.applyErr = domainErr
				}
				calls := 0
				service.SetObserver(func(actualOperation string, refs map[string]any, actualErr error) error {
					calls++
					wantOperation := "register"
					if operation == "publish" {
						wantOperation = "dataset-define"
					}
					if actualOperation != wantOperation || refs["authorityId"] != registry.AuthorityID() {
						t.Errorf("observer lost actual operation: %s %#v", actualOperation, refs)
					}
					if rejected && actualErr != domainErr || !rejected && actualErr != nil {
						t.Errorf("observer received wrong authority outcome: %v", actualErr)
					}
					if outcome == "rejected-observer-nil" {
						return nil
					}
					return recordErr
				})
				mutate := func() error {
					if operation == "register" {
						return service.RegisterRepository(other)
					}
					_, err := service.PublishDataset(candidate)
					return err
				}
				resultErr := mutate()
				wantErr := recordErr
				if rejected {
					wantErr = domainErr
				}
				if resultErr != wantErr || kernel.CodeOf(resultErr) != kernel.CodeOf(wantErr) {
					t.Errorf("observer replaced authority outcome: got %v, want original %v", resultErr, wantErr)
				}
				if calls != 1 {
					t.Fatalf("observer calls = %d, want one actual result", calls)
				}
				actualHead, err := authority.Head(snapshot.DefaultRef)
				if err != nil {
					t.Fatal(err)
				}
				acceptedHead, err := registry.Head()
				if err != nil || acceptedHead != string(actualHead) {
					t.Fatalf("handle does not reflect actual authority: %s %s %v", acceptedHead, actualHead, err)
				}
				after := catalog.NormalizeState(service.DumpState())
				if rejected {
					if acceptedHead != head || !reflect.DeepEqual(after, before) {
						t.Fatalf("rejected publication changed state or HEAD: %s -> %s, %#v", head, acceptedHead, after)
					}
				} else {
					if acceptedHead == head || reflect.DeepEqual(after, before) {
						t.Fatal("record error rolled back an accepted authority change")
					}
					if operation == "register" {
						if !service.HasRepository(other) {
							t.Fatal("accepted registration disappeared on record error")
						}
					} else if current, err := service.Set("delivery"); err != nil || current.Revision != 2 {
						t.Fatalf("accepted publication disappeared on record error: %#v %v", current, err)
					}
				}
				reopened, err := catalog.OpenSnapshotRegistry(authority, "")
				if err != nil {
					t.Fatal(err)
				}
				recovered := catalogFromRegistry(t, reopened, source, other)
				if got := catalog.NormalizeState(recovered.DumpState()); !reflect.DeepEqual(got, after) {
					t.Fatalf("reopen did not recover actual authority: got %#v, want %#v", got, after)
				}
				if historical, err := recovered.DatasetVersion("delivery", 1); err != nil || historical.Revision != 1 {
					t.Fatalf("observer outcome damaged prior release: %#v %v", historical, err)
				}
				if !rejected && operation == "publish" {
					if published, err := recovered.DatasetVersion("delivery", 2); err != nil || !reflect.DeepEqual(published, candidate) {
						t.Fatalf("accepted release not recoverable after record error: %#v %v", published, err)
					}
				}
				if !rejected {
					service.SetObserver(nil)
					retryErr := mutate()
					if operation == "register" && retryErr != nil || operation == "publish" && kernel.CodeOf(retryErr) != kernel.ErrNonFastForward {
						t.Fatalf("retry did not honor actual acceptance: %v", retryErr)
					}
					if got, err := authority.Head(snapshot.DefaultRef); err != nil || got != actualHead {
						t.Fatalf("retry duplicated accepted change: %s %v", got, err)
					}
				}
			})
		}
	}
}
