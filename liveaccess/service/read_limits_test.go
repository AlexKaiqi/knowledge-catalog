package service_test

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"kc/kernel"
	"kc/liveaccess"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
)

// slowLookup waits per unit and honors the unit's context like a real source.
type slowLookup struct {
	mu    sync.Mutex
	delay func(objectmodel.Address) time.Duration
	calls []objectmodel.Address
	base  unitLookup
}

func (s *slowLookup) LookupState(ctx context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	s.mu.Lock()
	s.calls = append(s.calls, request.Binding.Address)
	s.mu.Unlock()
	select {
	case <-time.After(s.delay(request.Binding.Address)):
		return s.base.LookupState(ctx, request)
	case <-ctx.Done():
		return liveaccess.StateObservation{}, ctx.Err()
	}
}

func timedOut(value objectmodel.ObservedValue, address objectmodel.Address) bool {
	for _, failure := range value.Failures {
		if failure.Address == address && failure.Code == kernel.ErrTemporaryUnavailable && strings.Contains(failure.Message, "timed out") {
			return true
		}
	}
	return false
}

func externalAspects(id objectmodel.ObjectID) []objectmodel.Address {
	var out []objectmodel.Address
	for _, aspect := range []string{"good", "empty", "broken"} {
		out = append(out, objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: aspect})
	}
	return out
}

func TestReadSharesRemainingTimeAcrossExternalUnits(t *testing.T) {
	base, _, _, id := mixedUnitFixture(t)
	lookup := &slowLookup{delay: func(objectmodel.Address) time.Duration { return 200 * time.Millisecond }}
	service := liveservice.Open(base, lookup, kernel.IdentityContext{Principal: "consumer"})
	// Each unit alone fits its own 1s limit; together they exceed the 300ms the read shares.
	ctx := liveaccess.ContextWithReadLimits(context.Background(), liveaccess.ReadLimits{Lookup: time.Second, Read: 300 * time.Millisecond})
	started := time.Now()
	values, err := service.Read(ctx, id, nil)
	elapsed := time.Since(started)
	if err != nil || len(values) != 1 {
		t.Fatalf("exhausted read budget failed the whole read: %#v %v", values, err)
	}
	result := values[0]
	if body := result.Value.(map[string]any); !reflect.DeepEqual(body["definition"], map[string]any{"owner": "fixed"}) {
		t.Fatalf("static aspect was lost when external units timed out: %#v", body)
	}
	var delivered, timeouts int
	for _, address := range externalAspects(id) {
		switch {
		case timedOut(result, address):
			timeouts++
		default:
			delivered++
		}
	}
	if delivered != 1 || timeouts != 2 || len(result.Observations) != 1 {
		t.Fatalf("want the first unit delivered and the other two timed out at their own addresses: delivered=%d timeouts=%d %#v", delivered, timeouts, result)
	}
	if len(lookup.calls) != 2 {
		t.Fatalf("a unit started after the read budget was used up: calls=%v", lookup.calls)
	}
	if elapsed > 600*time.Millisecond {
		t.Fatalf("read took %v; unit limits were summed instead of sharing the 300ms budget", elapsed)
	}
}

func TestUnitLookupLimitFailsOnlyThatUnit(t *testing.T) {
	base, _, _, id := mixedUnitFixture(t)
	broken := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "broken"}
	lookup := &slowLookup{delay: func(address objectmodel.Address) time.Duration {
		if address == broken {
			return 2 * time.Second
		}
		return 0
	}}
	service := liveservice.Open(base, lookup, kernel.IdentityContext{Principal: "consumer"})
	ctx := liveaccess.ContextWithReadLimits(context.Background(), liveaccess.ReadLimits{Lookup: 50 * time.Millisecond})
	started := time.Now()
	values, err := service.Read(ctx, id, nil)
	if err != nil || len(values) != 1 {
		t.Fatalf("one slow unit failed the whole read: %#v %v", values, err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("unit limit was not applied: %v", elapsed)
	}
	result := values[0]
	if !timedOut(result, broken) || len(result.Observations) != 2 || len(lookup.calls) != 3 {
		t.Fatalf("only the slow unit should time out, its siblings observed: %#v calls=%v", result, lookup.calls)
	}
	if body := result.Value.(map[string]any); !reflect.DeepEqual(body["good"], map[string]any{"value": float64(31)}) || body["empty"] != nil {
		t.Fatalf("siblings of the timed-out unit changed: %#v", body)
	}
}

func TestReadWithoutLimitsKeepsTransportCancellationFatal(t *testing.T) {
	base, _, _, id := mixedUnitFixture(t)
	lookup := &slowLookup{delay: func(objectmodel.Address) time.Duration { return time.Second }}
	service := liveservice.Open(base, lookup, kernel.IdentityContext{Principal: "consumer"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Read(ctx, id, nil); err == nil {
		t.Fatal("a cancelled request was reported as a partial success")
	}
}

func TestReadLimitsConfigRejectsNonPositiveDurations(t *testing.T) {
	limits, err := liveaccess.RuntimeConfig{LookupTimeout: "1s", ReadTimeout: "600ms"}.Limits()
	if err != nil || limits != (liveaccess.ReadLimits{Lookup: time.Second, Read: 600 * time.Millisecond}) {
		t.Fatalf("limits = %#v %v", limits, err)
	}
	for _, raw := range []string{"0s", "-1s", "soon"} {
		if err := (liveaccess.RuntimeConfig{ReadTimeout: raw}).Validate(); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("readTimeout %q accepted: %v", raw, err)
		}
	}
}
