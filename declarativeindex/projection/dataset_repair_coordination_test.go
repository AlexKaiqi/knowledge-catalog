package projection

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"kc/kernel"
	"kc/object/reader"
)

func TestNamedPreparationWaitsForAcceptedResourceRecovery(t *testing.T) {
	idx := NewIndexEngine("", nil)
	entered, release := make(chan struct{}), make(chan struct{})
	scoped := make(chan struct{})
	var maintenance atomic.Bool
	idx.SetPublishedDatasetRecovery(func(_ context.Context, setID string, revision int) error {
		if setID == "" {
			maintenance.Store(true)
			close(entered)
			<-release
			maintenance.Store(false)
			return nil
		}
		if setID != "named" || revision != 3 || maintenance.Load() {
			t.Errorf("preparation crossed active authority recovery or changed scope: %s/%d active=%v", setID, revision, maintenance.Load())
		}
		close(scoped)
		return errors.New("scope preparation stopped before source access")
	})
	background := make(chan error, 1)
	go func() { background <- idx.RecoverResources(context.Background()) }()
	<-entered
	prepared := make(chan error, 1)
	go func() {
		_, err := idx.PrepareDataset(context.Background(), nil, reader.DatasetPin{SetID: "named", Revision: 3})
		prepared <- err
	}()
	select {
	case <-scoped:
		t.Fatal("named preparation started while background authority repair remained active")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-background; err != nil {
		t.Fatal(err)
	}
	if err := <-prepared; err == nil || kernel.CodeOf(err) == kernel.ErrUsageInvalid {
		t.Fatalf("named recovery did not precede source preparation: %v", err)
	}
	<-scoped
}

func TestTemporaryPreparationDoesNotRecoverNamedPublication(t *testing.T) {
	idx := NewIndexEngine("", nil)
	idx.SetPublishedDatasetRecovery(func(context.Context, string, int) error {
		t.Fatal("temporary candidate entered named authority repair")
		return nil
	})
	_, err := idx.PrepareDataset(context.Background(), nil, reader.DatasetPin{Temporary: true})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("invalid temporary scope: %v", err)
	}
}

func TestCanceledPreparationDoesNotWaitForBackgroundAuthorityRecovery(t *testing.T) {
	idx := NewIndexEngine("", nil)
	entered, release := make(chan struct{}), make(chan struct{})
	idx.SetPublishedDatasetRecovery(func(_ context.Context, setID string, _ int) error {
		if setID != "" {
			t.Error("canceled preparation entered authority recovery")
			return nil
		}
		close(entered)
		<-release
		return nil
	})
	background := make(chan error, 1)
	go func() { background <- idx.RecoverResources(context.Background()) }()
	<-entered
	defer func() {
		close(release)
		if err := <-background; err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := idx.PrepareDataset(ctx, nil, reader.DatasetPin{SetID: "named", Revision: 1})
		result <- err
	}()
	select {
	case err := <-result:
		t.Fatalf("preparation bypassed active recovery: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled preparation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled request waited for background recovery to finish")
	}
}
