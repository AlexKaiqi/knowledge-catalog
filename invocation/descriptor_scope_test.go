package invocation

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestRequestDescriptorsNeverProbeUnreadableSources(t *testing.T) {
	private := newDescriptorScopeStore("kr://descriptor/private", descriptorScopeValues("secret"))
	f := newDescriptorScopeFixture(t, private)
	at, err := requestDescriptors(f.cx, f.primary.id, "v1")("ops/stats")
	if err != nil || at.Repository != f.ops.id {
		t.Fatalf("readable descriptor failed: %#v %v", at, err)
	}
	if heads, reads := private.counts("ops/stats"); heads != 0 || reads != 0 {
		t.Fatalf("unreadable source was contacted: heads=%d reads=%d", heads, reads)
	}
}

func TestRequestDescriptorsPinsVersionsAndCachesWithinOneRequest(t *testing.T) {
	f := newDescriptorScopeFixture(t)
	resolve := requestDescriptors(f.cx, f.primary.id, "v1")
	first, err := resolve("ops/stats")
	if err != nil || first.Commit != "v1" || first.Value != "one" {
		t.Fatalf("first declaration version: %#v %v", first, err)
	}
	f.ops.advance("v2", descriptorScopeValues("two"))
	for _, id := range []objectmodel.ObjectID{"ops/stats", "ops/monitor"} {
		at, err := resolve(id)
		if err != nil || at.Commit != first.Commit || at.Value != first.Value {
			t.Errorf("same request followed a moving source for %s: %#v %v", id, at, err)
		}
		if heads, reads := f.ops.counts(id); heads != 1 || reads != 1 {
			t.Errorf("fixed declaration was fetched again for %s: heads=%d reads=%d", id, heads, reads)
		}
	}
}

func TestRequestDescriptorsNewRequestRefreshesVersionAndAuthorization(t *testing.T) {
	f := newDescriptorScopeFixture(t)
	if _, err := requestDescriptors(f.cx, f.primary.id, "v1")("ops/stats"); err != nil {
		t.Fatal(err)
	}
	f.ops.advance("v2", descriptorScopeValues("two"))
	at, err := requestDescriptors(f.cx, f.primary.id, "v1")("ops/stats")
	if err != nil || at.Commit != "v2" || at.Value != "two" {
		t.Fatalf("independent request reused an older declaration: %#v %v", at, err)
	}
	f.allow(t, f.primary.id)
	_, err = requestDescriptors(f.cx, f.primary.id, "v1")("ops/stats")
	if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("new request retained revoked descriptor permission: %v", err)
	}
	if heads, reads := f.ops.counts("ops/stats"); heads != 2 || reads != 2 {
		t.Fatalf("revoked source was contacted: heads=%d reads=%d", heads, reads)
	}
}

func TestRequestDescriptorsKeepsObjectQualifiedPermissionsPerReference(t *testing.T) {
	f := newDescriptorScopeFixture(t)
	if err := WriteAllow(f.cx.Home, AllowFile{Rules: []AllowRule{
		{ID: "main", Principal: "reader", Actions: []string{"object.read"}, Repo: string(f.primary.id)},
		{ID: "stats", Principal: "reader", Actions: []string{"object.read"}, Repo: string(f.ops.id), Object: "ops/stats"},
	}}); err != nil {
		t.Fatal(err)
	}
	resolve := requestDescriptors(f.cx, f.primary.id, "v1")
	if at, err := resolve("ops/stats"); err != nil || at.Repository != f.ops.id {
		t.Fatalf("qualified permission did not resolve its descriptor: %#v %v", at, err)
	}
	if _, err := resolve("ops/monitor"); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("one descriptor's permission opened another: %v", err)
	}
	if heads, reads := f.ops.counts("ops/monitor"); heads != 1 || reads != 0 {
		t.Fatalf("unreadable descriptor accessed authority: heads=%d reads=%d", heads, reads)
	}
}

func TestRequestDescriptorsRejectsACandidateRemovedAfterScopeCreation(t *testing.T) {
	other := newDescriptorScopeStore("kr://descriptor/other", nil)
	f := newDescriptorScopeFixture(t, other)
	f.allow(t, f.primary.id, f.ops.id, other.id)
	resolve := requestDescriptors(f.cx, f.primary.id, "v1")
	f.cx.WS.Store.Delete(other.id)
	if at, err := resolve("ops/stats"); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved || at.Repository != "" {
		t.Fatalf("candidate disappearance silently proved uniqueness: %#v %v", at, err)
	}
}

func TestRequestDescriptorsFailClosedWhenAnAuthorizedCandidateIsUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*descriptorScopeStore)
		want kernel.ErrorCode
	}{
		{"ambiguous", func(s *descriptorScopeStore) { s.versions["v1"] = descriptorScopeValues("other") }, kernel.ErrObjectIDConflict},
		{"head-failure", func(s *descriptorScopeStore) {
			s.headErr = kernel.Fail(kernel.ErrTemporaryUnavailable, "head unavailable")
		}, kernel.ErrTemporaryUnavailable},
		{"unknown-head", func(s *descriptorScopeStore) { s.head = "" }, kernel.ErrVersionUnresolved},
		{"read-failure", func(s *descriptorScopeStore) {
			s.readErr = kernel.Fail(kernel.ErrTemporaryUnavailable, "descriptor unavailable")
		}, kernel.ErrTemporaryUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := newDescriptorScopeStore("kr://descriptor/other", nil)
			tc.edit(other)
			f := newDescriptorScopeFixture(t, other)
			f.allow(t, f.primary.id, f.ops.id, other.id)
			at, err := requestDescriptors(f.cx, f.primary.id, "v1")("ops/stats")
			if kernel.CodeOf(err) != tc.want || at.Repository != "" {
				t.Fatalf("unproven uniqueness silently picked a holder: %#v %v, want %s", at, err, tc.want)
			}
		})
	}
}

func TestRequestDescriptorsEmptyInitializedRepositoryAndLocalPrecedence(t *testing.T) {
	empty := newDescriptorScopeStore("kr://descriptor/empty", nil)
	f := newDescriptorScopeFixture(t, empty)
	f.allow(t, f.primary.id, f.ops.id, empty.id)
	at, err := requestDescriptors(f.cx, f.primary.id, "v1")("ops/stats")
	if err != nil || at.Repository != f.ops.id {
		t.Fatalf("an empty initialized repository prevented a unique descriptor: %#v %v", at, err)
	}
	f.primary.advance("local", descriptorScopeValues("local"))
	f.ops.headErr = kernel.Fail(kernel.ErrTemporaryUnavailable, "remote must not be reached")
	at, err = requestDescriptors(f.cx, f.primary.id, "local")("ops/stats")
	if err != nil || at.Repository != f.primary.id || at.Value != "local" {
		t.Fatalf("local fixed declaration lost precedence: %#v %v", at, err)
	}
	if heads, _ := f.ops.counts("ops/stats"); heads != 1 {
		t.Fatalf("local resolution probed remote head: %d", heads)
	}
}

func TestRequestDescriptorsConcurrentLookupsShareOneFixedDeclaration(t *testing.T) {
	f := newDescriptorScopeFixture(t)
	resolve := requestDescriptors(f.cx, f.primary.id, "v1")
	var workers sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			at, err := resolve("ops/stats")
			if err != nil || at.Commit != "v1" || at.Value != "one" {
				t.Errorf("concurrent declaration changed: %#v %v", at, err)
			}
		}()
	}
	close(start)
	workers.Wait()
	if heads, reads := f.ops.counts("ops/stats"); heads != 1 || reads != 1 {
		t.Fatalf("concurrent request repeated authority access: heads=%d reads=%d", heads, reads)
	}
}

// A read's descriptor resolution must not wait on every other Repository in
// turn: the Head probes overlap, and the overlap stays within the bound.
func TestRequestDescriptorsProbeOtherRepositoriesConcurrentlyWithinBound(t *testing.T) {
	const overlap = 4
	var mu sync.Mutex
	inFlight, peak := 0, 0
	released := make(chan struct{})
	var release sync.Once
	gate := func() {
		mu.Lock()
		inFlight++
		peak = max(peak, inFlight)
		if inFlight == overlap {
			release.Do(func() { close(released) })
		}
		mu.Unlock()
		select {
		case <-released:
		case <-time.After(2 * time.Second):
		}
		mu.Lock()
		inFlight--
		mu.Unlock()
	}
	others := []*descriptorScopeStore{}
	ids := []kernel.RepositoryID{}
	for i := 0; i < 2*descriptorScanWorkers; i++ {
		other := newDescriptorScopeStore(kernel.RepositoryID(fmt.Sprintf("kr://descriptor/other-%02d", i)), nil)
		other.gate = gate
		others, ids = append(others, other), append(ids, other.id)
	}
	f := newDescriptorScopeFixture(t, others...)
	f.ops.gate = gate
	f.allow(t, append([]kernel.RepositoryID{f.primary.id, f.ops.id}, ids...)...)
	at, err := requestDescriptors(f.cx, f.primary.id, "v1")("ops/stats")
	if err != nil || at.Repository != f.ops.id {
		t.Fatalf("concurrent probes changed the unique holder: %#v %v", at, err)
	}
	select {
	case <-released:
	default:
		t.Fatalf("other Repositories were probed one at a time: peak in-flight Head calls %d, want %d", peak, overlap)
	}
	if peak > descriptorScanWorkers {
		t.Fatalf("probe fan-out %d exceeded its bound %d", peak, descriptorScanWorkers)
	}
	for _, other := range others {
		if heads, reads := other.counts("ops/stats"); heads != 1 || reads != 1 {
			t.Fatalf("%s was not probed exactly once: heads=%d reads=%d", other.id, heads, reads)
		}
	}
}
