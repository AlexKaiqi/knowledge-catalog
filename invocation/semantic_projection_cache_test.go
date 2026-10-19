package invocation

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kc/kernel"
)

// A cold projection build must delay only requests for the same immutable
// key: another key is served while it runs, and same-key callers share the
// one build instead of building again.
func TestSemanticProjectionBuildDoesNotBlockOtherKeys(t *testing.T) {
	cache := newSemanticProjectionCache(32)
	slow := semanticProjectionKey{repository: "kr://projection/a", commit: kernel.CommitID("c1")}
	fast := semanticProjectionKey{repository: "kr://projection/b", commit: kernel.CommitID("c1")}
	entered, release := make(chan struct{}), make(chan struct{})
	var builds atomic.Int32
	want := &SemanticProjection{}
	slowBuild := func() (*SemanticProjection, error) {
		builds.Add(1)
		close(entered)
		<-release
		return want, nil
	}

	var wg sync.WaitGroup
	results := make(chan *SemanticProjection, 2)
	wg.Add(1)
	go func() { defer wg.Done(); got, _ := cache.get(slow, slowBuild); results <- got }()
	<-entered
	wg.Add(1)
	go func() {
		defer wg.Done()
		got, _ := cache.get(slow, func() (*SemanticProjection, error) { builds.Add(1); return nil, errors.New("second build") })
		results <- got
	}()

	done := make(chan error, 1)
	go func() {
		_, err := cache.get(fast, func() (*SemanticProjection, error) { return &SemanticProjection{}, nil })
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("another key waited for a cold projection build")
	}

	close(release)
	wg.Wait()
	close(results)
	for got := range results {
		if got != want {
			t.Fatalf("same-key caller got %p, want the shared build %p", got, want)
		}
	}
	if n := builds.Load(); n != 1 {
		t.Fatalf("same key built %d times, want 1", n)
	}
}

func TestSemanticProjectionFailedBuildIsRetried(t *testing.T) {
	cache := newSemanticProjectionCache(32)
	key := semanticProjectionKey{repository: "kr://projection/retry", commit: kernel.CommitID("c1")}
	if _, err := cache.get(key, func() (*SemanticProjection, error) { return nil, errors.New("transient") }); err == nil {
		t.Fatal("failed build reported success")
	}
	want := &SemanticProjection{}
	got, err := cache.get(key, func() (*SemanticProjection, error) { return want, nil })
	if err != nil || got != want {
		t.Fatalf("failed build was cached: %p %v", got, err)
	}
	if len(cache.order) != 1 || len(cache.entries) != 1 {
		t.Fatalf("cache bookkeeping drifted: order=%d entries=%d", len(cache.order), len(cache.entries))
	}
}
