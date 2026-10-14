package plugin_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	index "kc/discovery/projection"
	"kc/liveaccess"
	sourcebridge "kc/liveaccess/discovery"
)

func TestIndexControlSameKeyNoticeDuringObservationRemainsPending(t *testing.T) {
	for _, hints := range []string{"different-revision", "empty-revision"} {
		t.Run(hints, func(t *testing.T) {
			f := newControlRecoveryFixture(t)
			source := f.base.setups[0]
			at, address := controlBoundJob(t, f)
			runtime := f.open(t, source.RepositoryID)
			f.publish(t, runtime, "notice-race", source.RepositoryID, at)
			lookup := &controlStateLookup{status: "baseline", revision: "r0"}
			runtime.plugins.SetObservationSource(controlObservationSource(lookup))
			if err := runtime.plugins.CatchUp(context.Background()); err != nil {
				t.Fatal(err)
			}
			evidence := f.accepted(t)
			lookup.mu.Lock()
			lookup.status, lookup.revision = "first-observation", "r1"
			lookup.mu.Unlock()
			blocked := &controlNoticeRaceLookup{lookup: lookup, entered: make(chan struct{}), release: make(chan struct{})}
			workerSource := controlObservationSource(lookup).(sourcebridge.Source)
			workerSource.Lookup = blocked
			runtime.plugins.SetObservationSource(workerSource)
			firstHint, secondHint := "r1", "r2"
			if hints == "empty-revision" {
				firstHint, secondHint = "", ""
			}
			if err := runtime.plugins.Notify(index.ChangeNotice{Repository: source.RepositoryID, Address: &address, SourceRevision: firstHint}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(func() { blocked.releaseOnce.Do(func() { close(blocked.release) }); cancel() })
			finished := make(chan error, 1)
			go func() { finished <- runtime.plugins.CatchUp(ctx) }()
			select {
			case <-blocked.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("actual Controller notice did not reach the State source")
			}
			lookup.mu.Lock()
			lookup.status, lookup.revision = "second-observation", "r2"
			lookup.mu.Unlock()
			if err := runtime.plugins.Notify(index.ChangeNotice{Repository: source.RepositoryID, Address: &address, SourceRevision: secondHint}); err != nil {
				t.Fatal(err)
			}
			blocked.releaseOnce.Do(func() { close(blocked.release) })
			select {
			case err := <-finished:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("actual first observation did not finish")
			}
			assertControlState(t, f, runtime, lookup, at, "first-observation", "a-owner", "r1")
			if err := runtime.plugins.CatchUp(context.Background()); err != nil {
				t.Fatal(err)
			}
			if blocked.calls.Load() != 2 {
				t.Errorf("completion of the old notice discarded the newer same-key notice: actual source calls=%d", blocked.calls.Load())
			}
			assertControlState(t, f, runtime, lookup, at, "second-observation", "a-owner", "r2")
			f.assertAccepted(t, evidence)
		})
	}
}

type controlNoticeRaceLookup struct {
	lookup      *controlStateLookup
	entered     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	calls       atomic.Int64
}

func (s *controlNoticeRaceLookup) LookupState(ctx context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	observation, err := s.lookup.LookupState(ctx, request)
	if s.calls.Add(1) == 1 {
		close(s.entered)
		select {
		case <-s.release:
		case <-ctx.Done():
			return liveaccess.StateObservation{}, ctx.Err()
		}
	}
	return observation, err
}

var _ liveaccess.StateLookup = (*controlNoticeRaceLookup)(nil)
